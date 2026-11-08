package tibber

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/widget"
)

var berlin, _ = time.LoadLocation("Europe/Berlin")

// 2026-11-08 14:30 in Berlin (UTC+1).
var now = time.Date(2026, 11, 8, 14, 30, 0, 0, berlin)

type fake struct {
	srv        *httptest.Server
	tokenCalls atomic.Int32
	priceCalls atomic.Int32
	refreshes  []string
	tomorrow   bool // publish tomorrow's prices
	reject401  atomic.Bool
}

func item(t time.Time, price any) map[string]any {
	return map[string]any{"time": t.Format(time.RFC3339), "consumption": map[string]any{"price": price, "vatPercent": 19}}
}

func newFake(t *testing.T) *fake {
	f := &fake{tomorrow: true}
	mux := http.NewServeMux()
	mux.HandleFunc("/connect/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.tokenCalls.Add(1)
		f.refreshes = append(f.refreshes, r.Form.Get("refresh_token"))
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("client_id") != "cid" || r.Form.Get("client_secret") != "sec" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized_client"})
			return
		}
		if r.Form.Get("refresh_token") != "r1" && r.Form.Get("refresh_token") != "r2" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant"})
			return
		}
		next := "r2"
		if r.Form.Get("refresh_token") == "r2" {
			next = "r3"
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at" + next, "expires_in": 3600, "refresh_token": next})
	})
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if f.reject401.Load() || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer at") {
			w.WriteHeader(401)
			return false
		}
		return true
	}
	mux.HandleFunc("/v1/homes", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			json.NewEncoder(w).Encode(map[string]any{"homes": []map[string]any{{"id": "h1", "name": "Zuhause"}, {"id": "h2", "name": "Ferienhaus"}}})
		}
	})
	mux.HandleFunc("/v1/homes/", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.priceCalls.Add(1)
		since, err := time.Parse(time.RFC3339, r.URL.Query().Get("since"))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		// page 1: from since 48 h (192 slots, cut to 100 to force paging), page 2 (cursor): the rest
		var items []map[string]any
		start := since
		last := dayStart(now, berlin).AddDate(0, 0, 1) // end of today
		if f.tomorrow {
			last = last.AddDate(0, 0, 1)
		}
		for ts := start; ts.Before(last); ts = ts.Add(15 * time.Minute) {
			var price any = 0.30
			if ts.Hour() == 3 {
				price = "0.1234" // numeric strings are allowed by the schema
			}
			items = append(items, item(ts, price))
		}
		resp := map[string]any{"currency": "EUR"}
		if r.URL.Query().Get("cursor") == "" && len(items) > 100 {
			resp["items"] = items[:100]
			resp["next"] = "/v1/homes/" + strings.Split(r.URL.Path, "/")[3] + "/prices?cursor=2&since=" + url.QueryEscape(items[100]["time"].(string))
		} else {
			if r.URL.Query().Get("cursor") != "" {
				since, _ = time.Parse(time.RFC3339, r.URL.Query().Get("since"))
				var rest []map[string]any
				for _, it := range items {
					if tt, _ := time.Parse(time.RFC3339, it["time"].(string)); !tt.Before(since) {
						rest = append(rest, it)
					}
				}
				items = rest
			}
			resp["items"] = items
			if len(items) > 0 {
				resp["next"] = "/v1/homes/h1/prices?cursor=3&since=" + url.QueryEscape(last.Format(time.RFC3339))
			}
		}
		json.NewEncoder(w).Encode(resp)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func start(t *testing.T, f *fake, settings map[string]any, file string) *Provider {
	t.Helper()
	p := newProvider()
	p.apiURL, p.tokenURL, p.now = f.srv.URL, f.srv.URL+"/connect/token", func() time.Time { return now }
	err := p.Start(context.Background(), widget.Env{
		Settings:   settings,
		Connection: map[string]any{"clientId": "cid", "clientSecret": "sec", "refreshToken": "r1", "tokenFile": file},
		Location:   berlin,
	})
	if err != nil {
		t.Fatal(err)
	}
	// no sharing between tests
	p.ts = newTokenSource(p.client, p.tokenURL, "cid", "sec", "r1", file, p.now)
	return p
}

func TestFetchTodayAndTomorrow(t *testing.T) {
	f := newFake(t)
	p := start(t, f, nil, filepath.Join(t.TempDir(), "tok.json"))
	v, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d := v.(Data)
	if d.Label != "Zuhause" || d.Currency != "EUR" || d.StepSec != 900 || d.Timezone != "Europe/Berlin" {
		t.Fatalf("unexpected header: %+v", d)
	}
	if len(d.Slots) != 192 {
		t.Fatalf("want 192 slots (today + tomorrow), got %d", len(d.Slots))
	}
	if first := time.Unix(d.Slots[0].T, 0).In(berlin); first.Hour() != 0 || first.Day() != 8 {
		t.Fatalf("first slot should be today 00:00, got %v", first)
	}
	for i := 1; i < len(d.Slots); i++ {
		if d.Slots[i].T-d.Slots[i-1].T != 900 {
			t.Fatalf("slots not contiguous at %d", i)
		}
	}
	if d.Slots[12].Price != 0.1234 {
		t.Fatalf("string price not parsed: %v", d.Slots[12])
	}
	if f.priceCalls.Load() != 3 { // two pages with prices, then the empty "not published yet" page
		t.Fatalf("want 3 price requests, got %d", f.priceCalls.Load())
	}
}

func TestHomeBySettingAndLabel(t *testing.T) {
	f := newFake(t)
	p := start(t, f, map[string]any{"home": "ferienhaus", "label": "Strom"}, filepath.Join(t.TempDir(), "tok.json"))
	v, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.(Data).Label != "Strom" || p.homeID != "h2" {
		t.Fatalf("home/label: %q %q", v.(Data).Label, p.homeID)
	}
	p2 := start(t, f, map[string]any{"home": "nope"}, filepath.Join(t.TempDir(), "tok.json"))
	if _, err := p2.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), `no home "nope"`) {
		t.Fatalf("want unknown-home error, got %v", err)
	}
}

func TestCacheUntilTomorrowMissing(t *testing.T) {
	f := newFake(t)
	p := start(t, f, nil, filepath.Join(t.TempDir(), "tok.json"))
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := f.priceCalls.Load()
	if _, err := p.Fetch(context.Background()); err != nil || f.priceCalls.Load() != calls {
		t.Fatalf("complete data must be served from cache (calls %d -> %d, err %v)", calls, f.priceCalls.Load(), err)
	}

	// Tomorrow not published yet: retried after an hour, not before.
	f2 := newFake(t)
	f2.tomorrow = false
	p2 := start(t, f2, nil, filepath.Join(t.TempDir(), "tok.json"))
	if _, err := p2.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := f2.priceCalls.Load()
	p2.Fetch(context.Background())
	if f2.priceCalls.Load() != c {
		t.Fatal("must not refetch within the hour")
	}
	later := now.Add(61 * time.Minute)
	p2.now = func() time.Time { return later }
	f2.tomorrow = true
	v, err := p2.Fetch(context.Background())
	if err != nil || len(v.(Data).Slots) != 192 {
		t.Fatalf("refetch after an hour should find tomorrow: %v", err)
	}
}

func TestNewDayRefetches(t *testing.T) {
	f := newFake(t)
	p := start(t, f, nil, filepath.Join(t.TempDir(), "tok.json"))
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.stale(now.Add(time.Hour)) {
		t.Fatal("same day, complete: not stale")
	}
	if !p.stale(now.Add(10 * time.Hour)) {
		t.Fatal("next day: stale")
	}
}

func TestRefreshTokenRotationIsPersisted(t *testing.T) {
	f := newFake(t)
	file := filepath.Join(t.TempDir(), "sub", "tok.json")
	p := start(t, f, nil, file)
	if _, err := p.ts.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(b), `"refreshToken":"r2"`) {
		t.Fatalf("rotated token not saved: %s %v", b, err)
	}
	// "restart": the same config token continues with the saved one
	ts := newTokenSource(http.DefaultClient, f.srv.URL+"/connect/token", "cid", "sec", "r1", file, func() time.Time { return now })
	if ts.refresh != "r2" {
		t.Fatalf("saved token should win, got %s", ts.refresh)
	}
	// a new token in the config wins over the old file
	ts = newTokenSource(http.DefaultClient, f.srv.URL+"/connect/token", "cid", "sec", "fresh", file, func() time.Time { return now })
	if ts.refresh != "fresh" {
		t.Fatalf("new configured token should win, got %s", ts.refresh)
	}
}

func TestAccessTokenIsCachedAndRetriedOn401(t *testing.T) {
	f := newFake(t)
	p := start(t, f, nil, filepath.Join(t.TempDir(), "tok.json"))
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.tokenCalls.Load() != 1 {
		t.Fatalf("access token should be reused, token calls: %d", f.tokenCalls.Load())
	}
	p.cache = nil
	f.reject401.Store(true)
	if _, err := p.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want 401 error, got %v", err)
	}
	if f.tokenCalls.Load() != 2 {
		t.Fatalf("a 401 should trigger exactly one token refresh, got %d", f.tokenCalls.Load())
	}
}

func TestBadRefreshTokenError(t *testing.T) {
	f := newFake(t)
	p := start(t, f, nil, filepath.Join(t.TempDir(), "tok.json"))
	p.ts = newTokenSource(p.client, p.tokenURL, "cid", "sec", "dead", "", p.now)
	_, err := p.Fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid_grant") || strings.Contains(err.Error(), "dead") {
		t.Fatalf("want invalid_grant without the token in the message, got %v", err)
	}
}

func TestStartValidatesConnection(t *testing.T) {
	for name, c := range map[string]map[string]any{
		"none":      nil,
		"no id":     {"clientSecret": "s", "refreshToken": "r"},
		"no secret": {"clientId": "i", "refreshToken": "r"},
		"no token":  {"clientId": "i", "clientSecret": "s"},
	} {
		if err := newProvider().Start(context.Background(), widget.Env{Connection: c}); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
