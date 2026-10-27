// Package widget defines the contract between the core and widget providers
// (see docs/adr/004-widget-contract.md).
package widget

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/pmcgn/lumina/internal/history"
)

// Env is handed to a provider when it is started.
type Env struct {
	ID         string
	Settings   map[string]any
	Connection map[string]any // nil if the widget has no connection
	// Connections are all configured connections by name, for widgets that
	// reference several (e.g. one per calendar). Settings reach the browser,
	// connections never do: reference them by name only.
	Connections map[string]map[string]any
	Logger      *slog.Logger
	Location    *time.Location // configured timezone
	History     *history.Store // shared in-memory time series; prefix series names with ID
}

// Provider is the minimum every widget type implements. Start validates the
// settings and prepares the provider; an error marks only this widget as failed.
type Provider interface {
	Start(ctx context.Context, env Env) error
}

// Poller provides data by being asked periodically.
type Poller interface {
	Fetch(ctx context.Context) (any, error)
	Interval() time.Duration
}

// Streamer pushes data. Stream blocks until ctx is done or an error occurs.
type Streamer interface {
	Stream(ctx context.Context, emit func(any)) error
}

// Actor handles actions sent by the frontend (e.g. "toggle", "complete").
type Actor interface {
	Action(ctx context.Context, name string, params json.RawMessage) (any, error)
}

var (
	mu       sync.RWMutex
	registry = map[string]func() Provider{}
)

// Register makes a widget type available. Call it from the provider package's init().
func Register(typeID string, factory func() Provider) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[typeID]; dup {
		panic("widget: duplicate type " + typeID)
	}
	registry[typeID] = factory
}

// New creates a provider of the given type.
func New(typeID string) (Provider, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := registry[typeID]
	if !ok {
		return nil, false
	}
	return f(), true
}

// Types lists the registered widget types.
func Types() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// Settings helpers (settings come from YAML, so values are loosely typed).

func String(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

func Bool(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

func Int(m map[string]any, key string, def int) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return def
}
