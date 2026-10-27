// Package core contains the transport-independent heart of Lumina: it owns
// the widget runners, the connected clients and the subscription fan-out.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/pmcgn/lumina/internal/config"
	"github.com/pmcgn/lumina/internal/history"
)

const (
	clientQueue   = 64
	actionTimeout = 20 * time.Second
)

type Hub struct {
	log     *slog.Logger
	version string
	hist    *history.Store
	ctx     context.Context
	cancel  context.CancelFunc

	mu      sync.RWMutex
	cfg     *config.Config
	runners map[string]*runner
	prints  map[string]string // widget id -> fingerprint of its config
	clients map[*Client]struct{}
}

func NewHub(log *slog.Logger, version string) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{
		log:     log,
		version: version,
		hist:    history.New(0, 0),
		ctx:     ctx,
		cancel:  cancel,
		runners: map[string]*runner{},
		prints:  map[string]string{},
		clients: map[*Client]struct{}{},
	}
}

// Ready reports whether a configuration has been applied.
func (h *Hub) Ready() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg != nil
}

// Close stops all runners. Connected clients are disconnected by the transport.
func (h *Hub) Close() {
	h.cancel()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.runners {
		r.stop()
	}
}

// Apply activates a (validated) configuration. Widgets whose configuration did
// not change keep their runner and state; clients get the new config and keep
// their subscriptions where the widget still exists.
func (h *Hub) Apply(cfg *config.Config) {
	loc := time.Local
	if cfg.Timezone != "" {
		loc, _ = time.LoadLocation(cfg.Timezone) // validated by config
	}
	h.mu.Lock()
	runners := map[string]*runner{}
	prints := map[string]string{}
	for _, p := range cfg.Pages {
		for _, w := range p.Widgets {
			fp := fingerprint(w, cfg.Connections) + "|" + cfg.Timezone
			if old, ok := h.runners[w.ID]; ok && h.prints[w.ID] == fp {
				runners[w.ID] = old
			} else {
				runners[w.ID] = newRunner(h.ctx, w, cfg.Connections, h.hist, loc, h.log)
			}
			prints[w.ID] = fp
		}
	}
	for id, old := range h.runners {
		if runners[id] != old {
			old.stop()
		}
	}
	h.cfg, h.runners, h.prints = cfg, runners, prints
	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	for _, c := range clients {
		h.sendConfig(c)
		h.resubscribe(c)
	}
	h.log.Info("configuration applied", "widgets", len(runners), "clients", len(clients))
}

// fingerprint changes when the widget or any connection changes; widgets may
// reference connections from their settings, so all of them count.
func fingerprint(w config.Widget, conns map[string]map[string]any) string {
	a, _ := json.Marshal(w)
	b, _ := json.Marshal(conns)
	// json omits Connection and AlwaysOn (json:"-"), include them explicitly.
	return fmt.Sprintf("%s|%s|%s|%v", a, b, w.Connection, w.AlwaysOn)
}

func (h *Hub) sendConfig(c *Client) {
	h.mu.RLock()
	cfg := h.cfg
	h.mu.RUnlock()
	if cfg == nil {
		return
	}
	c.Send(encode(MsgConfig, "", "", cfg.Client()))
}

func (h *Hub) runner(id string) *runner {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.runners[id]
}

// resubscribe re-attaches a client to the current runners after a config change.
func (h *Hub) resubscribe(c *Client) {
	for _, id := range c.subList() {
		r := h.runner(id)
		if r == nil {
			c.removeSub(id)
			continue
		}
		r.attach(c)
	}
}

// NewClient registers a client. kick must disconnect the transport.
func (h *Hub) NewClient(kick func()) *Client {
	c := &Client{hub: h, out: make(chan []byte, clientQueue), kick: kick, subs: map[string]struct{}{}}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// RemoveClient unregisters a client and releases its subscriptions.
func (h *Hub) RemoveClient(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	runners := make([]*runner, 0, len(h.runners))
	for _, r := range h.runners {
		runners = append(runners, r)
	}
	h.mu.Unlock()

	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	for _, r := range runners {
		r.detach(c)
	}
}

// Handle processes one decoded message from a client.
func (h *Hub) Handle(c *Client, msg Envelope) {
	switch msg.Type {
	case MsgHello:
		c.Send(encode(MsgWelcome, "", "", WelcomePayload{ServerVersion: h.version, Protocol: ProtocolVersion}))
		h.sendConfig(c)
	case MsgSubscribe:
		var p SubscribePayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			c.Send(encode(MsgError, "", msg.ID, map[string]string{"message": "bad subscribe payload"}))
			return
		}
		for _, id := range p.Widgets {
			r := h.runner(id)
			if r == nil {
				c.Send(encode(MsgError, id, msg.ID, map[string]string{"message": "unknown widget"}))
				continue
			}
			if c.addSub(id) {
				r.attach(c)
			}
		}
	case MsgUnsubscribe:
		var p SubscribePayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return
		}
		for _, id := range p.Widgets {
			if c.removeSub(id) {
				if r := h.runner(id); r != nil {
					r.detach(c)
				}
			}
		}
	case MsgPing:
		c.Send(encode(MsgPong, "", msg.ID, nil))
	case MsgAction:
		go h.handleAction(c, msg)
	default:
		c.Send(encode(MsgError, "", msg.ID, map[string]string{"message": "unknown message type " + msg.Type}))
	}
}
