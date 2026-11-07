package core

import (
	"encoding/json"
	"time"
)

// ProtocolVersion is the version of the WebSocket envelope (docs/adr/003).
const ProtocolVersion = 1

// Envelope is the single message format in both directions.
type Envelope struct {
	V       int             `json:"v"`
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Widget  string          `json:"widget,omitempty"`
	TS      int64           `json:"ts"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type SubscribePayload struct {
	Widgets []string `json:"widgets"`
}

type ActionPayload struct {
	Name   string          `json:"name"`
	Params json.RawMessage `json:"params,omitempty"`
}

type ActionResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

type WelcomePayload struct {
	ServerVersion string `json:"serverVersion"`
	Protocol      int    `json:"protocol"`
}

// WidgetStatus is the state of a widget's data.
type WidgetStatus string

const (
	WidgetStatusLoading WidgetStatus = "loading" // no data yet
	WidgetStatusOK      WidgetStatus = "ok"
	WidgetStatusStale   WidgetStatus = "stale" // last fetch failed, older data is shown
	WidgetStatusError   WidgetStatus = "error" // failed and no data available
)

// WidgetState is the payload of snapshot and update messages.
type WidgetState struct {
	Status    WidgetStatus    `json:"status"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     string          `json:"error,omitempty"`
	UpdatedAt int64           `json:"updatedAt,omitempty"` // unix ms of the last successful fetch
}

// AlertColor is the color of an alert overlay; only theme colors are allowed.
type AlertColor string

const (
	AlertColorRed    AlertColor = "red"
	AlertColorOrange AlertColor = "orange"
)

// Alert is the payload of an alert message: an urgent notice shown over
// everything. RemainingMs is relative so that the browser's clock does not matter.
type Alert struct {
	ID          string     `json:"id"`
	Text        string     `json:"text"`
	Color       AlertColor `json:"color"`
	RemainingMs int64      `json:"remainingMs"`
	Confirmable bool       `json:"confirmable"` // the overlay offers a confirm button
}

// AlertRef names an alert: the payload of alert_clear (server) and alert_confirm (client).
type AlertRef struct {
	ID string `json:"id"`
}

func encode(typ, widget, id string, payload any) []byte {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return encode(MsgError, widget, id, map[string]string{"message": "encode: " + err.Error()})
		}
		raw = b
	}
	b, _ := json.Marshal(Envelope{V: ProtocolVersion, Type: typ, ID: id, Widget: widget, TS: time.Now().UnixMilli(), Payload: raw})
	return b
}
