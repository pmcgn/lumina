package core

import "time"

// webhookDelay is how long a webhook waits before the widgets are fetched: it
// lets the source finish committing and merges bursts of events.
const webhookDelay = time.Second

// Connection returns the options of a configured connection (nil, false if it
// does not exist). The map must not be modified.
func (h *Hub) Connection(name string) (map[string]any, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.cfg == nil {
		return nil, false
	}
	c, ok := h.cfg.Connections[name]
	return c, ok
}

// RefreshConnection makes every widget of the given type that uses the
// connection fetch fresh data soon (an incoming webhook says "something
// changed"; the data itself always comes from the regular fetch, so nothing can
// be applied twice). It returns the number of widgets that were asked.
func (h *Hub) RefreshConnection(connection, widgetType string) int {
	h.mu.RLock()
	var rs []*runner
	for _, r := range h.runners {
		if r.conf.Connection == connection && r.conf.Type == widgetType && r.startErr == nil {
			rs = append(rs, r)
		}
	}
	h.mu.RUnlock()
	for _, r := range rs {
		r.scheduleRefresh(webhookDelay)
	}
	return len(rs)
}
