package server_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pmcgn/lumina/internal/config"
	"github.com/pmcgn/lumina/internal/core"
	"github.com/pmcgn/lumina/internal/server"
	"github.com/pmcgn/lumina/internal/widget"
)

// fakeTasks stands in for the vikunja.tasks widget: it counts its fetches and
// would only poll once an hour, so every extra fetch comes from a webhook.
type fakeTasks struct{}

var fetches atomic.Int64

func (fakeTasks) Start(context.Context, widget.Env) error { return nil }
func (fakeTasks) Fetch(context.Context) (any, error) {
	return map[string]int64{"n": fetches.Add(1)}, nil
}
func (fakeTasks) Interval() time.Duration { return time.Hour }

func init() {
	widget.Register("vikunja.tasks", func() widget.Provider { return fakeTasks{} })
}

const webhookYAML = `
connections:
  open: {}
  signed: { webhookSecret: s3cret }
  basic: { webhookUser: lumina, webhookPassword: pw }
  both: { webhookSecret: s3cret, webhookUser: lumina, webhookPassword: pw }
pages:
  - id: p
    widgets:
      - { id: t-open, type: vikunja.tasks, connection: open, alwaysOn: true, pos: {x: 0, y: 0} }
      - { id: t-signed, type: vikunja.tasks, connection: signed, alwaysOn: true, pos: {x: 1, y: 0} }
`

func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func webhookServer(t *testing.T) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	cfg, err := config.Parse([]byte(webhookYAML), func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	hub := core.NewHub(log, "test")
	hub.Apply(cfg)
	t.Cleanup(hub.Close)
	srv := httptest.NewServer(server.New(hub, log, fstest.MapFS{"index.html": {Data: []byte("hi")}}).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, srv *httptest.Server, conn string, body []byte, mod func(*http.Request)) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/webhooks/vikunja/"+conn, bytes.NewReader(body))
	if mod != nil {
		mod(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestWebhookAuth(t *testing.T) {
	srv := webhookServer(t)
	body := []byte(`{"event_name":"task.updated","data":{}}`)
	sig := func(secret string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("X-Vikunja-Signature", sign(secret, body)) }
	}
	basic := func(u, p string) func(*http.Request) {
		return func(r *http.Request) { r.SetBasicAuth(u, p) }
	}
	both := func(secret, u, p string) func(*http.Request) {
		return func(r *http.Request) { sig(secret)(r); basic(u, p)(r) }
	}

	cases := []struct {
		name string
		conn string
		mod  func(*http.Request)
		want int
	}{
		{"unknown connection", "nope", nil, http.StatusNotFound},
		{"no auth configured", "open", nil, http.StatusNoContent},
		{"signature missing", "signed", nil, http.StatusUnauthorized},
		{"signature wrong", "signed", sig("other"), http.StatusUnauthorized},
		{"signature garbage", "signed", func(r *http.Request) { r.Header.Set("X-Vikunja-Signature", "zz") }, http.StatusUnauthorized},
		{"signature ok", "signed", sig("s3cret"), http.StatusNoContent},
		{"signature with prefix", "signed", func(r *http.Request) { r.Header.Set("X-Vikunja-Signature", "sha256="+sign("s3cret", body)) }, http.StatusNoContent},
		{"basic missing", "basic", nil, http.StatusUnauthorized},
		{"basic wrong password", "basic", basic("lumina", "x"), http.StatusUnauthorized},
		{"basic ok", "basic", basic("lumina", "pw"), http.StatusNoContent},
		{"both: only basic", "both", basic("lumina", "pw"), http.StatusUnauthorized},
		{"both: only signature", "both", sig("s3cret"), http.StatusUnauthorized},
		{"both ok", "both", both("s3cret", "lumina", "pw"), http.StatusNoContent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := post(t, srv, c.conn, body, c.mod); got != c.want {
				t.Fatalf("status = %d, want %d", got, c.want)
			}
		})
	}

	// The signature covers the body: a signature of another body is rejected.
	if got := post(t, srv, "signed", []byte(`{"tampered":true}`), sig("s3cret")); got != http.StatusUnauthorized {
		t.Fatalf("tampered body: status = %d, want 401", got)
	}
	// Basic auth challenge for clients that expect one.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/webhooks/vikunja/basic", strings.NewReader("{}"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("WWW-Authenticate"), "Basic ") {
		t.Fatalf("WWW-Authenticate = %q", resp.Header.Get("WWW-Authenticate"))
	}
}

// A burst of webhooks makes the widgets fetch again once, not once per event,
// and a rejected webhook triggers nothing.
func TestWebhookTriggersOneRefresh(t *testing.T) {
	srv := webhookServer(t)
	waitFor := func(cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatal("timeout")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitFor(func() bool { return fetches.Load() >= 2 }) // initial fetch of both alwaysOn widgets
	time.Sleep(100 * time.Millisecond)
	base := fetches.Load()

	if got := post(t, srv, "signed", []byte("{}"), nil); got != http.StatusUnauthorized {
		t.Fatalf("status = %d", got)
	}
	time.Sleep(1500 * time.Millisecond)
	if n := fetches.Load(); n != base {
		t.Fatalf("rejected webhook caused %d fetches", n-base)
	}

	for i := 0; i < 5; i++ {
		if got := post(t, srv, "open", []byte("{}"), nil); got != http.StatusNoContent {
			t.Fatalf("status = %d", got)
		}
	}
	waitFor(func() bool { return fetches.Load() > base })
	time.Sleep(1500 * time.Millisecond)
	if n := fetches.Load() - base; n != 1 {
		t.Fatalf("5 webhooks caused %d fetches, want 1 (only the widget of the connection)", n)
	}
}
