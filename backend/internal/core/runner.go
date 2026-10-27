package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/pmcgn/lumina/internal/config"
	"github.com/pmcgn/lumina/internal/history"
	"github.com/pmcgn/lumina/internal/widget"
)

const (
	fetchTimeout    = 20 * time.Second
	coalesceWindow  = 250 * time.Millisecond
	maxBackoff      = 5 * time.Minute
	defaultInterval = time.Minute
)

// runner owns one widget instance: it fetches data while at least one client
// is subscribed (or the widget is alwaysOn), keeps the last state, detects
// changes and fans updates out to subscribers.
type runner struct {
	conf config.Widget
	prov widget.Provider
	log  *slog.Logger
	root context.Context

	mu       sync.Mutex
	subs     map[*Client]struct{}
	cancel   context.CancelFunc // non-nil while the loop is running
	state    WidgetState
	hash     [32]byte
	hasState bool
	lastSent time.Time
	flushing bool
	refresh  chan struct{}
	startErr error
	stopped  bool
}

func newRunner(root context.Context, conf config.Widget, conns map[string]map[string]any, hist *history.Store, loc *time.Location, log *slog.Logger) *runner {
	r := &runner{
		conf:    conf,
		log:     log.With("widget", conf.ID, "type", conf.Type),
		root:    root,
		subs:    map[*Client]struct{}{},
		refresh: make(chan struct{}, 1),
		state:   WidgetState{Status: WidgetStatusLoading},
	}
	prov, _ := widget.New(conf.Type)
	r.prov = prov
	err := prov.Start(root, widget.Env{ID: conf.ID, Settings: conf.Settings, Connection: conns[conf.Connection], Connections: conns, Logger: r.log, History: hist, Location: loc})
	if err != nil {
		r.startErr = err
		r.state = WidgetState{Status: WidgetStatusError, Error: err.Error()}
		r.log.Error("widget failed to start", "err", err)
	}
	if conf.AlwaysOn {
		r.mu.Lock()
		r.startLoopLocked()
		r.mu.Unlock()
	}
	return r
}

func (r *runner) snapshotMsg() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return encode(MsgSnapshot, r.conf.ID, "", r.state)
}

func (r *runner) attach(c *Client) {
	r.mu.Lock()
	r.subs[c] = struct{}{}
	r.startLoopLocked()
	msg := encode(MsgSnapshot, r.conf.ID, "", r.state)
	r.mu.Unlock()
	c.Send(msg)
}

func (r *runner) detach(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subs, c)
	if len(r.subs) == 0 && !r.conf.AlwaysOn && r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}

func (r *runner) stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}

// requestRefresh asks the poll loop to fetch immediately (e.g. after an action).
func (r *runner) requestRefresh() {
	select {
	case r.refresh <- struct{}{}:
	default:
	}
}

func (r *runner) startLoopLocked() {
	if r.cancel != nil || r.stopped || r.startErr != nil {
		return
	}
	ctx, cancel := context.WithCancel(r.root)
	r.cancel = cancel
	go r.loop(ctx)
}

func (r *runner) loop(ctx context.Context) {
	switch p := r.prov.(type) {
	case widget.Streamer:
		r.streamLoop(ctx, p)
	case widget.Poller:
		r.pollLoop(ctx, p)
	}
}

func (r *runner) pollLoop(ctx context.Context, p widget.Poller) {
	failures := 0
	for {
		data, err := safeFetch(ctx, p)
		var wait time.Duration
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			failures++
			r.setError(err)
			wait = backoff(interval(p), failures)
			r.log.Warn("fetch failed", "err", err, "retry_in", wait)
		} else {
			failures = 0
			r.setData(data)
			wait = interval(p)
			wait += time.Duration(rand.Int63n(int64(wait/10) + 1)) // jitter
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-r.refresh:
			t.Stop()
		case <-t.C:
		}
	}
}

func (r *runner) streamLoop(ctx context.Context, p widget.Streamer) {
	failures := 0
	for ctx.Err() == nil {
		started := time.Now()
		err := safeStream(ctx, p, r.setData)
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > time.Minute {
			failures = 0 // it ran for a while, so this is a fresh failure
		}
		failures++
		if err != nil {
			r.setError(err)
		}
		wait := backoff(5*time.Second, failures)
		r.log.Warn("stream ended", "err", err, "retry_in", wait)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func interval(p widget.Poller) time.Duration {
	if d := p.Interval(); d > 0 {
		return d
	}
	return defaultInterval
}

func backoff(base time.Duration, failures int) time.Duration {
	d := base
	for i := 1; i < failures && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}

// safeFetch isolates provider panics and enforces a timeout.
func safeFetch(ctx context.Context, p widget.Poller) (data any, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic in provider: %v", rec)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	return p.Fetch(ctx)
}

func safeStream(ctx context.Context, p widget.Streamer, emit func(any)) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic in provider: %v", rec)
		}
	}()
	return p.Stream(ctx, emit)
}

func (r *runner) setData(data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		r.setError(fmt.Errorf("encode data: %w", err))
		return
	}
	r.publish(WidgetState{Status: WidgetStatusOK, Data: raw, UpdatedAt: time.Now().UnixMilli()}, true)
}

func (r *runner) setError(err error) {
	r.mu.Lock()
	prev := r.state
	r.mu.Unlock()
	next := WidgetState{Status: WidgetStatusError, Error: err.Error()}
	if len(prev.Data) > 0 {
		next = WidgetState{Status: WidgetStatusStale, Data: prev.Data, Error: err.Error(), UpdatedAt: prev.UpdatedAt}
	}
	r.publish(next, false)
}

// publish stores the new state and notifies subscribers if something visible
// changed. Notifications are coalesced to at most one per coalesceWindow.
func (r *runner) publish(next WidgetState, fresh bool) {
	// The timestamp is not part of the change detection, otherwise every poll
	// would produce an update.
	h := sha256.Sum256([]byte(string(next.Status) + "\x00" + next.Error + "\x00" + string(next.Data)))

	r.mu.Lock()
	if r.hasState && h == r.hash {
		if fresh {
			r.state.UpdatedAt = next.UpdatedAt
		}
		r.mu.Unlock()
		return
	}
	r.state, r.hash, r.hasState = next, h, true

	wait := coalesceWindow - time.Since(r.lastSent)
	if wait <= 0 {
		r.flushLocked()
		r.mu.Unlock()
		return
	}
	if !r.flushing {
		r.flushing = true
		time.AfterFunc(wait, func() {
			r.mu.Lock()
			r.flushing = false
			r.flushLocked()
			r.mu.Unlock()
		})
	}
	r.mu.Unlock()
}

func (r *runner) flushLocked() {
	r.lastSent = time.Now()
	if len(r.subs) == 0 {
		return
	}
	msg := encode(MsgUpdate, r.conf.ID, "", r.state)
	for c := range r.subs {
		c.Send(msg)
	}
}
