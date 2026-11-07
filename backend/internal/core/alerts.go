package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxAlertText    = 600
	maxAlertSeconds = 24 * 60 * 60
	maxActiveAlerts = 10
	callbackTimeout = 10 * time.Second
)

var (
	// ErrAlertNotFound is returned when an alert already expired or was confirmed.
	ErrAlertNotFound = errors.New("alert not found")
	errAlertBusy     = errors.New("alert is being confirmed")
)

// AlertRequest is the body of the REST call that triggers an alert.
type AlertRequest struct {
	Text            string     `json:"text"`
	Color           AlertColor `json:"color"`
	DurationSeconds int        `json:"durationSeconds"`
	// CallbackURL is optional. When set, the overlay shows a confirm button and
	// the backend calls this URL (POST) once somebody pressed it.
	CallbackURL string `json:"callbackUrl,omitempty"`
}

// alertEntry is an active alert plus what must never reach the browser.
type alertEntry struct {
	id         string
	text       string
	color      AlertColor
	expires    time.Time
	callback   string
	timer      *time.Timer
	confirming bool
}

func (a *alertEntry) wire(now time.Time) Alert {
	return Alert{
		ID:          a.id,
		Text:        a.text,
		Color:       a.color,
		RemainingMs: max(a.expires.Sub(now).Milliseconds(), 0),
		Confirmable: a.callback != "",
	}
}

// Validate checks and normalizes a request. The error text is meant for the caller.
func (r *AlertRequest) Validate() error {
	r.Text = strings.TrimSpace(r.Text)
	switch {
	case r.Text == "":
		return errors.New(`"text" is required`)
	case utf8.RuneCountInString(r.Text) > maxAlertText:
		return fmt.Errorf(`"text" is too long (max %d characters)`, maxAlertText)
	}
	if r.Color != AlertColorRed && r.Color != AlertColorOrange {
		return errors.New(`"color" must be "red" or "orange"`)
	}
	if r.DurationSeconds < 1 || r.DurationSeconds > maxAlertSeconds {
		return fmt.Errorf(`"durationSeconds" must be between 1 and %d`, maxAlertSeconds)
	}
	if r.CallbackURL != "" {
		u, err := url.Parse(r.CallbackURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New(`"callbackUrl" must be an absolute http(s) URL`)
		}
	}
	return nil
}

// TriggerAlert shows an alert on every connected client (and on those that
// connect while it is active). It returns the alert id and when it expires.
func (h *Hub) TriggerAlert(req AlertRequest) (string, time.Time, error) {
	if err := req.Validate(); err != nil {
		return "", time.Time{}, err
	}
	var raw [8]byte
	rand.Read(raw[:])
	now := time.Now()
	a := &alertEntry{
		id:       hex.EncodeToString(raw[:]),
		text:     req.Text,
		color:    req.Color,
		expires:  now.Add(time.Duration(req.DurationSeconds) * time.Second),
		callback: req.CallbackURL,
	}
	a.timer = time.AfterFunc(time.Until(a.expires), func() { h.clearAlert(a.id) })

	h.alertMu.Lock()
	h.alerts = append(h.alerts, a)
	var evicted *alertEntry
	if len(h.alerts) > maxActiveAlerts { // a flood must not pile up forever: the oldest goes
		evicted = h.alerts[0]
		h.alerts = h.alerts[1:]
		evicted.timer.Stop()
	}
	h.alertMu.Unlock()

	if evicted != nil {
		h.broadcast(encode(MsgAlertClear, "", "", AlertRef{ID: evicted.id}))
	}
	h.broadcast(encode(MsgAlert, "", "", a.wire(now)))
	h.log.Info("alert triggered", "id", a.id, "color", a.color, "seconds", req.DurationSeconds, "confirm", a.callback != "")
	return a.id, a.expires, nil
}

// clearAlert removes an alert (expired, dismissed or confirmed) and tells the clients.
func (h *Hub) clearAlert(id string) bool {
	h.alertMu.Lock()
	found := false
	for i, a := range h.alerts {
		if a.id == id {
			a.timer.Stop()
			h.alerts = append(h.alerts[:i:i], h.alerts[i+1:]...)
			found = true
			break
		}
	}
	h.alertMu.Unlock()
	if found {
		h.broadcast(encode(MsgAlertClear, "", "", AlertRef{ID: id}))
	}
	return found
}

// DismissAlert removes an alert without calling its callback.
func (h *Hub) DismissAlert(id string) error {
	if !h.clearAlert(id) {
		return ErrAlertNotFound
	}
	return nil
}

// ConfirmAlert calls the callback URL stored on the server (never one sent by
// a browser) and, if that worked, removes the alert everywhere. When the call
// fails the alert stays visible so it can be confirmed again.
func (h *Hub) ConfirmAlert(ctx context.Context, id string) error {
	h.alertMu.Lock()
	var a *alertEntry
	for _, e := range h.alerts {
		if e.id == id {
			a = e
			break
		}
	}
	switch {
	case a == nil:
		h.alertMu.Unlock()
		return ErrAlertNotFound
	case a.callback == "":
		h.alertMu.Unlock()
		return errors.New("alert has no confirm action")
	case a.confirming:
		h.alertMu.Unlock()
		return errAlertBusy
	}
	a.confirming = true
	callback := a.callback
	h.alertMu.Unlock()

	err := callCallback(ctx, callback, id)
	if err != nil {
		h.alertMu.Lock()
		a.confirming = false
		h.alertMu.Unlock()
		h.log.Warn("alert callback failed", "id", id, "err", err)
		return errors.New("callback failed")
	}
	h.clearAlert(id)
	h.log.Info("alert confirmed", "id", id)
	return nil
}

// callCallback POSTs a small JSON body. Errors never contain the URL: it may
// carry a token.
func callCallback(ctx context.Context, rawURL, id string) error {
	ctx, cancel := context.WithTimeout(ctx, callbackTimeout)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"alertId": id, "confirmed": true, "confirmedAt": time.Now().UTC().Format(time.RFC3339)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "lumina")
	client := &http.Client{
		Timeout:       callbackTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("POST %s: %w", req.URL.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("POST %s: HTTP %d", req.URL.Host, resp.StatusCode)
	}
	return nil
}

// sendAlerts hands a newly connected client the alerts that are still active.
func (h *Hub) sendAlerts(c *Client) {
	now := time.Now()
	h.alertMu.Lock()
	msgs := make([][]byte, 0, len(h.alerts))
	for _, a := range h.alerts {
		msgs = append(msgs, encode(MsgAlert, "", "", a.wire(now)))
	}
	h.alertMu.Unlock()
	for _, m := range msgs {
		c.Send(m)
	}
}

func (h *Hub) broadcast(msg []byte) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	for _, c := range clients {
		c.Send(msg)
	}
}

func (h *Hub) handleAlertConfirm(c *Client, msg Envelope) {
	var p AlertRef
	if err := json.Unmarshal(msg.Payload, &p); err != nil || p.ID == "" {
		c.Send(encode(MsgActionResult, "", msg.ID, ActionResult{Error: "bad alert payload"}))
		return
	}
	res := ActionResult{OK: true}
	if err := h.ConfirmAlert(h.ctx, p.ID); err != nil {
		res = ActionResult{Error: err.Error()}
	}
	c.Send(encode(MsgActionResult, "", msg.ID, res))
}
