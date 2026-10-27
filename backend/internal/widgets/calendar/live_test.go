package calendar

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/widget"
)

// TestLive loads a real iCal feed (e.g. a Google "secret address" or a public
// calendar) and prints what the widget would show. Skipped unless CALENDAR_URL
// is set. The URL is a secret for private calendars: pass it via the
// environment only and never log it.
func TestLive(t *testing.T) {
	url := os.Getenv("CALENDAR_URL")
	if url == "" {
		t.Skip("CALENDAR_URL not set")
	}
	loc, _ := time.LoadLocation("Europe/Berlin")
	p := &Provider{client: &http.Client{Timeout: 30 * time.Second}, now: time.Now}
	if err := p.Start(context.Background(), widget.Env{
		Settings:    map[string]any{"calendars": []any{"live"}, "days": 60, "limit": 12},
		Connections: map[string]map[string]any{"live": {"url": url}},
		Location:    loc,
	}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	v, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d := v.(Data)
	t.Logf("loaded in %v: %d agenda events, %d marked days, calendar status %+v", time.Since(start).Round(time.Millisecond), len(d.Events), len(d.Marks), d.Calendars)
	for _, e := range d.Events {
		when := time.Unix(e.Start, 0).In(loc).Format("Mon 02.01. 15:04")
		if e.AllDay {
			when = time.Unix(e.Start, 0).In(loc).Format("Mon 02.01.") + " (ganztägig)"
		}
		t.Logf("  %-28s %s", when, e.Title)
	}
	// A second fetch must be cheaper (conditional request / cache) and identical.
	v2, err := p.Fetch(context.Background())
	if err != nil || len(v2.(Data).Events) != len(d.Events) {
		t.Errorf("second fetch differs: %v", err)
	}
}
