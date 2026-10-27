// Package sse is a minimal Server-Sent Events client.
package sse

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Event is one dispatched SSE message.
type Event struct {
	Name string // "message" if the stream gave none
	Data string
}

// NewClient returns an HTTP client suitable for long-lived streams: it limits
// connecting and waiting for headers but not the body (which never ends).
func NewClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		ResponseHeaderTimeout: 15 * time.Second,
	}}
}

// ErrIdle is returned when the stream delivered no data for the idle period.
var ErrIdle = errors.New("sse: stream idle, connection presumed dead")

// Connect opens the stream and calls onEvent for every event until the stream
// ends, ctx is cancelled, or nothing (not even a comment/heartbeat line) is
// received for idle. onOpen is called once the response headers arrived.
func Connect(ctx context.Context, c *http.Client, url string, header http.Header, idle time.Duration, onOpen func(), onEvent func(Event)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sse: HTTP %d", resp.StatusCode)
	}

	var idled atomic.Bool
	watchdog := time.AfterFunc(idle, func() { idled.Store(true); cancel() })
	defer watchdog.Stop()
	if onOpen != nil {
		onOpen()
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	var name string
	var data []string
	for sc.Scan() {
		watchdog.Reset(idle)
		line := sc.Text()
		switch {
		case line == "": // dispatch
			if len(data) > 0 {
				ev := Event{Name: name, Data: strings.Join(data, "\n")}
				if ev.Name == "" {
					ev.Name = "message"
				}
				onEvent(ev)
			}
			name, data = "", nil
		case strings.HasPrefix(line, ":"): // comment / heartbeat
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if idled.Load() {
		return ErrIdle
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("sse: stream closed by server")
}
