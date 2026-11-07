package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"

	"github.com/pmcgn/lumina/internal/config"
	"github.com/pmcgn/lumina/internal/core"
	"github.com/pmcgn/lumina/internal/server"
	"github.com/pmcgn/lumina/internal/widget"
)

// counter is a provider whose value changes on every fetch and which supports
// a "bump" action.
type counter struct{ n atomic.Int64 }

func (c *counter) Start(context.Context, widget.Env) error { return nil }
func (c *counter) Fetch(context.Context) (any, error)      { return map[string]int64{"n": c.n.Load()}, nil }
func (c *counter) Interval() time.Duration                 { return 50 * time.Millisecond }
func (c *counter) Action(_ context.Context, name string, _ json.RawMessage) (any, error) {
	if name != "bump" {
		return nil, errors.New("unknown action")
	}
	c.n.Add(1)
	return nil, nil
}

func init() {
	widget.Register("test.counter", func() widget.Provider { return &counter{} })
}

const cfgYAML = `
pages:
  - id: p
    widgets:
      - { id: rw, type: test.counter, pos: {x: 0, y: 0} }
      - { id: ro, type: test.counter, readOnly: true, pos: {x: 1, y: 0} }
`

func setup(t *testing.T) (*websocket.Conn, context.Context) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	cfg, err := config.Parse([]byte(cfgYAML), func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	hub := core.NewHub(log, "test")
	hub.Apply(cfg)
	t.Cleanup(hub.Close)
	srv := httptest.NewServer(server.New(hub, log, fstest.MapFS{"index.html": {Data: []byte("hi")}}).Handler())
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn, ctx
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

func send(t *testing.T, ctx context.Context, c *websocket.Conn, typ, widget, id string, payload any) {
	t.Helper()
	p, _ := json.Marshal(payload)
	b, _ := json.Marshal(core.Envelope{V: 1, Type: typ, Widget: widget, ID: id, Payload: p})
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

// next reads messages until one of the wanted type (and widget, if given) arrives.
func next(t *testing.T, ctx context.Context, c *websocket.Conn, typ, widget string) core.Envelope {
	t.Helper()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %s/%s: %v", typ, widget, err)
		}
		var e core.Envelope
		if err := json.Unmarshal(data, &e); err != nil {
			t.Fatal(err)
		}
		if e.Type == typ && (widget == "" || e.Widget == widget) {
			return e
		}
	}
}

func TestSubscribeUpdateAndActions(t *testing.T) {
	conn, ctx := setup(t)

	send(t, ctx, conn, core.MsgHello, "", "", map[string]string{})
	w := next(t, ctx, conn, core.MsgWelcome, "")
	var wp core.WelcomePayload
	json.Unmarshal(w.Payload, &wp)
	if wp.ServerVersion != "test" {
		t.Errorf("version = %q", wp.ServerVersion)
	}
	cfg := next(t, ctx, conn, core.MsgConfig, "")
	if strings.Contains(string(cfg.Payload), "connection") {
		t.Errorf("config must not leak connections: %s", cfg.Payload)
	}

	send(t, ctx, conn, core.MsgSubscribe, "", "", core.SubscribePayload{Widgets: []string{"rw"}})
	snap := next(t, ctx, conn, core.MsgSnapshot, "rw")
	var st core.WidgetState
	json.Unmarshal(snap.Payload, &st)
	if st.Status != core.WidgetStatusLoading && st.Status != core.WidgetStatusOK {
		t.Errorf("initial status = %s", st.Status)
	}

	// Action on a writable widget succeeds and the new value is pushed.
	send(t, ctx, conn, core.MsgAction, "rw", "a1", core.ActionPayload{Name: "bump"})
	res := next(t, ctx, conn, core.MsgActionResult, "rw")
	var ar core.ActionResult
	json.Unmarshal(res.Payload, &ar)
	if res.ID != "a1" || !ar.OK {
		t.Fatalf("action result: %+v %s", res, res.Payload)
	}
	for {
		u := next(t, ctx, conn, core.MsgUpdate, "rw")
		json.Unmarshal(u.Payload, &st)
		if strings.Contains(string(st.Data), `"n":1`) {
			break
		}
	}

	// Read-only widget rejects actions.
	send(t, ctx, conn, core.MsgAction, "ro", "a2", core.ActionPayload{Name: "bump"})
	res = next(t, ctx, conn, core.MsgActionResult, "ro")
	json.Unmarshal(res.Payload, &ar)
	if ar.OK || !strings.Contains(ar.Error, "read-only") {
		t.Errorf("read-only action result: %s", res.Payload)
	}
}

func TestCrossOriginRejected(t *testing.T) {
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	hub := core.NewHub(log, "test")
	defer hub.Close()
	srv := httptest.NewServer(server.New(hub, log, fstest.MapFS{}).Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Origin": {"https://evil.example"}},
	})
	if err == nil {
		t.Fatal("cross-origin websocket must be rejected")
	}
}

func postAlert(t *testing.T, base, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(base+"/api/alerts", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func setupAlerts(t *testing.T) (base string, conn *websocket.Conn, ctx context.Context) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	hub := core.NewHub(log, "test")
	t.Cleanup(hub.Close)
	srv := httptest.NewServer(server.New(hub, log, fstest.MapFS{}).Handler())
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	send(t, ctx, conn, core.MsgHello, "", "", map[string]string{})
	next(t, ctx, conn, core.MsgWelcome, "")
	return srv.URL, conn, ctx
}

func TestAlertBroadcastConfirmAndExpiry(t *testing.T) {
	base, conn, ctx := setupAlerts(t)

	var calls atomic.Int32
	cb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer cb.Close()

	status, out := postAlert(t, base, `{"text":"Water leak","color":"red","durationSeconds":30,"callbackUrl":"`+cb.URL+`/ack?token=secret"}`)
	if status != http.StatusCreated {
		t.Fatalf("status = %d %v", status, out)
	}
	e := next(t, ctx, conn, core.MsgAlert, "")
	if strings.Contains(string(e.Payload), "secret") || strings.Contains(string(e.Payload), "callbackUrl") {
		t.Errorf("callback URL leaked to the browser: %s", e.Payload)
	}
	var a core.Alert
	json.Unmarshal(e.Payload, &a)
	if a.ID != out["id"] || a.Text != "Water leak" || a.Color != core.AlertColorRed || !a.Confirmable || a.RemainingMs <= 0 {
		t.Fatalf("alert = %+v", a)
	}

	// Confirming calls the stored URL, clears the alert for everybody.
	send(t, ctx, conn, core.MsgAlertConfirm, "", "c1", core.AlertRef{ID: a.ID})
	next(t, ctx, conn, core.MsgAlertClear, "") // sent to everybody before the reply to the confirming client
	var res core.ActionResult
	json.Unmarshal(next(t, ctx, conn, core.MsgActionResult, "").Payload, &res)
	if !res.OK || calls.Load() != 1 {
		t.Fatalf("confirm: %+v, callback calls = %d", res, calls.Load())
	}

	// Confirming again: the alert is gone.
	send(t, ctx, conn, core.MsgAlertConfirm, "", "c2", core.AlertRef{ID: a.ID})
	json.Unmarshal(next(t, ctx, conn, core.MsgActionResult, "").Payload, &res)
	if res.OK {
		t.Error("second confirm must fail")
	}

	// An alert without callback expires on its own.
	postAlert(t, base, `{"text":"Short","color":"orange","durationSeconds":1}`)
	json.Unmarshal(next(t, ctx, conn, core.MsgAlert, "").Payload, &a)
	if a.Confirmable {
		t.Error("no callback: no confirm button")
	}
	next(t, ctx, conn, core.MsgAlertClear, "")
}

func TestAlertFailingCallbackKeepsAlert(t *testing.T) {
	base, conn, ctx := setupAlerts(t)
	cb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) }))
	defer cb.Close()

	postAlert(t, base, `{"text":"x","color":"red","durationSeconds":30,"callbackUrl":"`+cb.URL+`"}`)
	var a core.Alert
	json.Unmarshal(next(t, ctx, conn, core.MsgAlert, "").Payload, &a)
	send(t, ctx, conn, core.MsgAlertConfirm, "", "c1", core.AlertRef{ID: a.ID})
	var res core.ActionResult
	json.Unmarshal(next(t, ctx, conn, core.MsgActionResult, "").Payload, &res)
	if res.OK || strings.Contains(res.Error, cb.URL) {
		t.Fatalf("result = %+v", res)
	}

	// Late joiners see the alert that is still active; DELETE removes it.
	conn2, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(base, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.CloseNow()
	send(t, ctx, conn2, core.MsgHello, "", "", map[string]string{})
	if got := next(t, ctx, conn2, core.MsgAlert, ""); !strings.Contains(string(got.Payload), a.ID) {
		t.Errorf("late joiner got %s", got.Payload)
	}
	req, _ := http.NewRequest(http.MethodDelete, base+"/api/alerts/"+a.ID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %v %v", err, resp)
	}
	next(t, ctx, conn, core.MsgAlertClear, "")
}

func TestAlertValidation(t *testing.T) {
	base, _, _ := setupAlerts(t)
	for name, body := range map[string]string{
		"no text":       `{"color":"red","durationSeconds":5}`,
		"bad color":     `{"text":"x","color":"green","durationSeconds":5}`,
		"no duration":   `{"text":"x","color":"red"}`,
		"huge duration": `{"text":"x","color":"red","durationSeconds":99999999}`,
		"bad callback":  `{"text":"x","color":"red","durationSeconds":5,"callbackUrl":"file:///etc/passwd"}`,
		"unknown field": `{"text":"x","colour":"red","durationSeconds":5}`,
		"not json":      `hello`,
		"too long":      `{"text":"` + strings.Repeat("a", 601) + `","color":"red","durationSeconds":5}`,
	} {
		if status, out := postAlert(t, base, body); status != http.StatusBadRequest || out["error"] == nil {
			t.Errorf("%s: status %d %v", name, status, out)
		}
	}
}
