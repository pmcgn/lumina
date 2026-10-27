// Package openhab is the openHAB widget. It shows a sitemap natively and lets
// you operate its switches. Structure and formatted labels come from the REST
// sitemap page; live changes come from the item event stream (SSE), which
// triggers a throttled re-read of the page, so values are always formatted
// exactly as openHAB does (units, patterns, transformations).
package openhab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pmcgn/lumina/internal/httpx"
	"github.com/pmcgn/lumina/internal/sse"
	"github.com/pmcgn/lumina/internal/widget"
)

const Type = "openhab.sitemap"

const (
	idleLimit       = 60 * time.Second // openHAB sends heartbeats far more often
	refreshThrottle = 300 * time.Millisecond
	fullRefresh     = 5 * time.Minute // safety net for missed events or sitemap edits
	maxTopicItems   = 100             // above this, subscribe to all items and filter locally
)

func init() {
	widget.Register(Type, func() widget.Provider {
		return &Provider{client: httpx.NewClient(), stream: sse.NewClient()}
	})
}

type Provider struct {
	client *http.Client // normal requests (with timeout)
	stream *http.Client // event stream (no body timeout)

	base    string
	header  http.Header
	sitemap string
	page    string
	title   string

	mu       sync.Mutex
	controls map[string]control
	items    map[string]bool
}

func (p *Provider) Start(_ context.Context, env widget.Env) error {
	c := env.Connection
	if c == nil {
		return errors.New("connection: this widget needs a connection with url")
	}
	raw := strings.TrimRight(widget.String(c, "url", ""), "/")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("connection.url: %q is not a valid http(s) URL", raw)
	}
	p.base = raw
	p.header = http.Header{}
	if token := widget.String(c, "token", ""); token != "" {
		p.header.Set("Authorization", "Bearer "+token)
	}
	s, err := parseSettings(env.Settings)
	if err != nil {
		return err
	}
	p.sitemap = s.Sitemap
	p.page = s.Page
	if p.page == "" {
		p.page = s.Sitemap
	}
	p.title = s.Title
	return nil
}

// parseSettings decodes and validates the widget settings.
func parseSettings(m map[string]any) (Settings, error) {
	var s Settings
	if err := widget.DecodeSettings(m, &s); err != nil {
		return s, err
	}
	if s.Sitemap == "" {
		return s, fmt.Errorf("setting %q is required (name of the openHAB sitemap)", "sitemap")
	}
	return s, nil
}

func (p *Provider) ValidateSettings(m map[string]any, _ map[string]map[string]any) error {
	_, err := parseSettings(m)
	return err
}

// load reads the sitemap page and rebuilds the row model.
func (p *Provider) load(ctx context.Context) (Data, error) {
	var page pageResp
	path := "/rest/sitemaps/" + url.PathEscape(p.sitemap) + "/" + url.PathEscape(p.page)
	if err := httpx.DoJSON(ctx, p.client, http.MethodGet, p.base+path, p.header, nil, &page); err != nil {
		return Data{}, err
	}
	rows, controls, items := buildRows(page)

	set := make(map[string]bool, len(items))
	for _, i := range items {
		set[i] = true
	}
	p.mu.Lock()
	p.controls, p.items = controls, set
	p.mu.Unlock()

	d := Data{Title: p.title, Rows: rows}
	if d.Title == "" {
		d.Title = page.Title
	}
	if d.Rows == nil {
		d.Rows = []Row{}
	}
	return d, nil
}

func (p *Provider) itemNames() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.items))
	for i := range p.items {
		out = append(out, i)
	}
	return out
}

func (p *Provider) relevant(item string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.items[item]
}

// eventsURL subscribes only to state changes of items shown in the sitemap.
func (p *Provider) eventsURL(items []string) string {
	topics := "openhab/items/*/statechanged"
	if len(items) > 0 && len(items) <= maxTopicItems {
		parts := make([]string, len(items))
		for i, it := range items {
			parts[i] = "openhab/items/" + it + "/statechanged"
		}
		topics = strings.Join(parts, ",")
	}
	return p.base + "/rest/events?topics=" + url.QueryEscape(topics)
}

// Stream implements widget.Streamer. It returns on any connection problem; the
// core restarts it with backoff, which re-reads everything (nothing is missed).
func (p *Provider) Stream(ctx context.Context, emit func(any)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	data, err := p.load(ctx)
	if err != nil {
		return err
	}
	emit(data)

	changed := make(chan struct{}, 1)
	poke := func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
	streamErr := make(chan error, 1)
	go func() {
		streamErr <- sse.Connect(ctx, p.stream, p.eventsURL(p.itemNames()), p.header, idleLimit,
			poke, // re-read once connected, closing the gap between load and subscribe
			func(ev sse.Event) {
				if item, ok := stateChangedItem(ev); ok && p.relevant(item) {
					poke()
				}
			})
	}()

	tick := time.NewTicker(fullRefresh)
	defer tick.Stop()
	refresh := func() error {
		d, err := p.load(ctx)
		if err != nil {
			return err
		}
		emit(d)
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-streamErr:
			return err
		case <-tick.C:
			if err := refresh(); err != nil {
				return err
			}
		case <-changed:
			// Throttle: collect a burst of changes into one re-read.
			select {
			case <-time.After(refreshThrottle):
			case <-ctx.Done():
				return ctx.Err()
			}
			select {
			case <-changed:
			default:
			}
			if err := refresh(); err != nil {
				return err
			}
		}
	}
}

// stateChangedItem extracts the item name from an item state-changed event.
func stateChangedItem(ev sse.Event) (string, bool) {
	var m struct {
		Topic string `json:"topic"`
	}
	if json.Unmarshal([]byte(ev.Data), &m) != nil {
		return "", false
	}
	parts := strings.Split(m.Topic, "/") // openhab/items/<name>/statechanged
	if len(parts) != 4 || parts[0] != "openhab" || parts[1] != "items" || parts[3] != "statechanged" {
		return "", false
	}
	return parts[2], true
}

// Action implements widget.Actor. Only "command" exists: it operates a switch
// or a mapping button of the rendered sitemap. Items that are not on the page
// cannot be commanded.
func (p *Provider) Action(ctx context.Context, name string, params json.RawMessage) (any, error) {
	if name != "command" {
		return nil, fmt.Errorf("unknown action %q", name)
	}
	var a struct {
		Widget  string `json:"widget"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal(params, &a); err != nil || a.Widget == "" || a.Command == "" {
		return nil, errors.New("command: widget and command required")
	}
	p.mu.Lock()
	c, ok := p.controls[a.Widget]
	p.mu.Unlock()
	if !ok {
		return nil, errors.New("command: unknown or not controllable widget")
	}
	if c.locked {
		return nil, errors.New("command: item is read-only in openHAB")
	}

	cmd := a.Command
	switch c.kind {
	case RowKindSwitch:
		switch cmd {
		case "TOGGLE":
			cmd = "ON"
			if c.on {
				cmd = "OFF"
			}
		case "ON", "OFF":
		default:
			return nil, fmt.Errorf("command: %q not allowed for a switch", cmd)
		}
	case RowKindSlider:
		var err error
		if cmd, err = sliderCommand(c, cmd); err != nil {
			return nil, err
		}
	case RowKindChoice:
		allowed := false
		for _, m := range c.mappings {
			if m.Command == cmd {
				allowed = true
			}
		}
		if !allowed {
			return nil, fmt.Errorf("command: %q is not one of the sitemap's mappings", cmd)
		}
	}
	endpoint := p.base + "/rest/items/" + url.PathEscape(c.item)
	if err := httpx.PostText(ctx, p.client, endpoint, p.header, cmd); err != nil {
		return nil, err
	}
	return map[string]string{"sent": cmd}, nil // the new state arrives through the event stream
}

// sliderCommand validates a command for a slider: a number within the
// sitemap's range (rounded to its step), or - depending on the item type -
// UP/DOWN/STOP for blinds and ON/OFF for dimmers and color items.
func sliderCommand(c control, cmd string) (string, error) {
	switch cmd {
	case "UP", "DOWN", "STOP":
		if c.itemType == "Rollershutter" {
			return cmd, nil
		}
	case "ON", "OFF":
		if c.itemType == "Dimmer" || c.itemType == "Color" {
			return cmd, nil
		}
	}
	v, err := strconv.ParseFloat(cmd, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return "", fmt.Errorf("command: %q not allowed for this slider", cmd)
	}
	if v < c.min || v > c.max {
		return "", fmt.Errorf("command: %v outside the allowed range %v..%v", v, c.min, c.max)
	}
	v = c.min + math.Round((v-c.min)/c.step)*c.step
	v = math.Round(v*1e6) / 1e6 // no float noise like 0.30000000000000004
	if v > c.max {
		v = c.max
	}
	return strconv.FormatFloat(v, 'f', -1, 64), nil
}
