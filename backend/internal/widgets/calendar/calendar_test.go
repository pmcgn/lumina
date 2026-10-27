package calendar

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/widget"
)

// Sat 17 Oct 2026 12:00 Berlin.
var now = time.Date(2026, 10, 17, 12, 0, 0, 0, berlin)

// feed is a fake calendar server for one ICS document.
type feed struct {
	mu       sync.Mutex
	body     string
	etag     string
	status   int // 0 = serve normally
	requests int
	ifNone   []string // If-None-Match headers received
}

func (f *feed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	f.ifNone = append(f.ifNone, r.Header.Get("If-None-Match"))
	if f.status != 0 {
		http.Error(w, "boom", f.status)
		return
	}
	if f.etag != "" {
		if r.Header.Get("If-None-Match") == f.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", f.etag)
	}
	w.Header().Set("Content-Type", "text/calendar")
	fmt.Fprint(w, f.body)
}

func newFeed(t *testing.T, events ...string) (*feed, string) {
	f := &feed{body: string(cal(events...))}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv.URL + "/ical/secret-token-abc123/basic.ics"
}

func timed(uid, title, start, end string) string {
	return ev("UID:"+uid, "SUMMARY:"+title, "DTSTART;TZID=Europe/Berlin:"+start, "DTEND;TZID=Europe/Berlin:"+end)
}

func allDay(uid, title, start, end string) string {
	return ev("UID:"+uid, "SUMMARY:"+title, "DTSTART;VALUE=DATE:"+start, "DTEND;VALUE=DATE:"+end)
}

func startProvider(t *testing.T, conns map[string]map[string]any, settings map[string]any) *Provider {
	t.Helper()
	p := &Provider{client: &http.Client{Timeout: 5 * time.Second}, now: func() time.Time { return now }}
	if err := p.Start(context.Background(), widget.Env{Settings: settings, Connections: conns, Location: berlin}); err != nil {
		t.Fatal(err)
	}
	return p
}

func fetch(t *testing.T, p *Provider) Data {
	t.Helper()
	v, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return v.(Data)
}

func titles(es []Event) string {
	var out []string
	for _, e := range es {
		out = append(out, e.Title)
	}
	return strings.Join(out, " | ")
}

func TestMergesCalendars(t *testing.T) {
	_, famURL := newFeed(t,
		timed("1", "Zahnarzt", "20261019T143000", "20261019T153000"),
		allDay("2", "Geburtstag Oma", "20261019", "20261020"),
	)
	_, workURL := newFeed(t,
		timed("3", "Standup", "20261019T090000", "20261019T093000"),
	)
	conns := map[string]map[string]any{"family": {"url": famURL}, "work": {"url": workURL}}
	p := startProvider(t, conns, map[string]any{"calendars": []any{
		map[string]any{"connection": "family", "name": "Familie", "color": "#ff8800"},
		"work",
	}})
	d := fetch(t, p)

	if got := titles(d.Events); got != "Geburtstag Oma | Standup | Zahnarzt" {
		t.Errorf("order (all-day first, then by time): %s", got)
	}
	if len(d.Calendars) != 2 || d.Calendars[0] != (Info{Name: "Familie", Color: "#ff8800"}) || d.Calendars[1].Name != "work" || d.Calendars[1].Color == "" {
		t.Errorf("calendars: %+v", d.Calendars)
	}
	if d.Events[0].Cal != 0 || d.Events[1].Cal != 1 || d.Events[2].Cal != 0 {
		t.Errorf("calendar index per event: %+v", d.Events)
	}
	if d.Timezone != "Europe/Berlin" || d.WeekStart != 1 {
		t.Errorf("meta: %+v", d)
	}
}

func TestAgendaWindowAndLimit(t *testing.T) {
	var evs []string
	evs = append(evs,
		timed("old", "Yesterday", "20261016T100000", "20261016T110000"),
		timed("morning", "Earlier today", "20261017T080000", "20261017T090000"), // already over, still part of today
		timed("far", "Beyond window", "20261101T100000", "20261101T110000"),
	)
	for i := 0; i < 20; i++ {
		evs = append(evs, timed(fmt.Sprint("e", i), fmt.Sprintf("Event %02d", i), fmt.Sprintf("202610%02dT100000", 18+i%10), fmt.Sprintf("202610%02dT110000", 18+i%10)))
	}
	_, u := newFeed(t, evs...)
	p := startProvider(t, map[string]map[string]any{"c": {"url": u}}, map[string]any{"calendars": []any{"c"}, "days": 14, "limit": 5})
	d := fetch(t, p)

	if len(d.Events) != 5 {
		t.Fatalf("limit not applied: %d", len(d.Events))
	}
	if d.Events[0].Title != "Earlier today" {
		t.Errorf("events of today must be included even if over: %s", titles(d.Events))
	}
	for _, e := range d.Events {
		if e.Title == "Yesterday" || e.Title == "Beyond window" {
			t.Errorf("outside the window: %s", e.Title)
		}
	}
}

func TestMonthMarks(t *testing.T) {
	_, a := newFeed(t,
		timed("1", "A", "20261019T100000", "20261019T110000"),
		timed("2", "Late", "20261020T230000", "20261021T000000"), // ends at midnight: only the 20th
		allDay("3", "Trip", "20261023", "20261026"),              // 23, 24, 25
		allDay("4", "Out of grid", "20261215", "20261216"),
		allDay("5", "Previous month in grid", "20260929", "20260930"), // grid starts Mon 28 Sep
	)
	_, b := newFeed(t, timed("6", "B", "20261019T120000", "20261019T130000"))
	p := startProvider(t, map[string]map[string]any{"a": {"url": a}, "b": {"url": b}}, map[string]any{"calendars": []any{"a", "b"}})
	d := fetch(t, p)

	if d.MonthFrom != "2026-09-28" {
		t.Errorf("grid starts on the Monday before 1 Oct: %s", d.MonthFrom)
	}
	check := func(day string, want ...int) {
		t.Helper()
		got := d.Marks[day]
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("marks[%s] = %v, want %v", day, got, want)
		}
	}
	check("2026-10-19", 0, 1)
	check("2026-10-20", 0)
	check("2026-10-21") // event ended exactly at midnight
	check("2026-10-23", 0)
	check("2026-10-24", 0)
	check("2026-10-25", 0)
	check("2026-10-26")
	check("2026-09-29", 0)
	if _, ok := d.Marks["2026-12-15"]; ok {
		t.Error("days outside the grid must not be marked")
	}

	// Sunday-first weeks move the grid start.
	p = startProvider(t, map[string]map[string]any{"a": {"url": a}}, map[string]any{"calendars": []any{"a"}, "weekStart": "sunday"})
	if got := fetch(t, p).MonthFrom; got != "2026-09-27" {
		t.Errorf("sunday grid start: %s", got)
	}
}

func TestConditionalRequestAndCacheFallback(t *testing.T) {
	f, u := newFeed(t, timed("1", "Termin", "20261019T100000", "20261019T110000"))
	f.etag = `"v1"`
	p := startProvider(t, map[string]map[string]any{"c": {"url": u}}, map[string]any{"calendars": []any{"c"}})

	if got := titles(fetch(t, p).Events); got != "Termin" {
		t.Fatalf("first fetch: %s", got)
	}
	d := fetch(t, p) // second request is conditional and answered with 304
	if titles(d.Events) != "Termin" || d.Calendars[0].Error != "" {
		t.Fatalf("after 304: %+v", d)
	}
	if f.ifNone[1] != `"v1"` {
		t.Errorf("must send If-None-Match, got %q", f.ifNone[1])
	}

	// Server breaks: cached data is still shown, the calendar is flagged.
	f.mu.Lock()
	f.status = http.StatusInternalServerError
	f.mu.Unlock()
	d = fetch(t, p)
	if titles(d.Events) != "Termin" {
		t.Errorf("cache must keep the agenda alive: %+v", d.Events)
	}
	if !strings.Contains(d.Calendars[0].Error, "500") {
		t.Errorf("calendar must be flagged: %+v", d.Calendars[0])
	}
}

func TestSecretURLNeverLeaks(t *testing.T) {
	// A server that is down: the transport error would normally contain the full URL.
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL + "/ical/secret-token-abc123/basic.ics"
	srv.Close()

	p := startProvider(t, map[string]map[string]any{"c": {"url": u}}, map[string]any{"calendars": []any{"c"}})
	_, err := p.Fetch(context.Background())
	if err == nil {
		t.Fatal("all calendars failing must be an error")
	}
	if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "/ical/") {
		t.Errorf("error leaks the secret feed url: %v", err)
	}

	// Invalid URL in the config must not be echoed either.
	err = (&Provider{}).Start(context.Background(), widget.Env{
		Settings:    map[string]any{"calendars": []any{"c"}},
		Connections: map[string]map[string]any{"c": {"url": "ftp://example.com/secret-token-abc123"}},
	})
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Errorf("config error must not echo the url: %v", err)
	}
}

func TestOneBrokenCalendarDoesNotHideTheOthers(t *testing.T) {
	_, good := newFeed(t, timed("1", "Termin", "20261019T100000", "20261019T110000"))
	bad, badURL := newFeed(t)
	bad.status = http.StatusForbidden
	p := startProvider(t, map[string]map[string]any{"good": {"url": good}, "bad": {"url": badURL}}, map[string]any{"calendars": []any{"good", "bad"}})
	d := fetch(t, p)
	if titles(d.Events) != "Termin" || d.Calendars[0].Error != "" || !strings.Contains(d.Calendars[1].Error, "403") {
		t.Errorf("%+v / %+v", d.Events, d.Calendars)
	}
}

func TestFileSourceAndWebcal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "home.ics")
	if err := os.WriteFile(path, cal(timed("1", "Aus Datei", "20261019T100000", "20261019T110000")), 0o600); err != nil {
		t.Fatal(err)
	}
	p := startProvider(t, map[string]map[string]any{"f": {"file": path}}, map[string]any{"calendars": []any{"f"}})
	if got := titles(fetch(t, p).Events); got != "Aus Datei" {
		t.Errorf("file source: %s", got)
	}

	src, err := parseSource(CalendarRef{Connection: "w"}, 0, map[string]map[string]any{"w": {"url": "webcal://example.com/cal.ics"}})
	if err != nil || src.url != "https://example.com/cal.ics" {
		t.Errorf("webcal must become https: %v %v", src, err)
	}
}

func TestStartValidation(t *testing.T) {
	conn := map[string]map[string]any{"c": {"url": "https://example.com/c.ics"}}
	for name, c := range map[string]struct {
		settings map[string]any
		conns    map[string]map[string]any
	}{
		"no calendars":       {map[string]any{}, conn},
		"empty list":         {map[string]any{"calendars": []any{}}, conn},
		"unknown connection": {map[string]any{"calendars": []any{"nope"}}, conn},
		"bad entry":          {map[string]any{"calendars": []any{42}}, conn},
		"no url or file":     {map[string]any{"calendars": []any{"c"}}, map[string]map[string]any{"c": {}}},
		"both url and file":  {map[string]any{"calendars": []any{"c"}}, map[string]map[string]any{"c": {"url": "https://x.example/a.ics", "file": "/a.ics"}}},
		"days too large":     {map[string]any{"calendars": []any{"c"}, "days": 365}, conn},
		"bad week start":     {map[string]any{"calendars": []any{"c"}, "weekStart": "tuesday"}, conn},
	} {
		if err := (&Provider{}).Start(context.Background(), widget.Env{Settings: c.settings, Connections: c.conns}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParallelFetchesAreSafe(t *testing.T) {
	// Two widgets (two providers) parse concurrently; the library's global zone
	// mapper must not mix up their fallback zones.
	_, u := newFeed(t, ev("UID:1", "SUMMARY:Floating", "DTSTART:20261019T100000", "DTEND:20261019T110000"))
	var wg sync.WaitGroup
	var bad atomic.Int32
	for _, name := range []string{"Europe/Berlin", "America/New_York", "Asia/Tokyo"} {
		loc := mustLoc(name)
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := &Provider{client: &http.Client{Timeout: 5 * time.Second}, now: func() time.Time { return now }}
			if err := p.Start(context.Background(), widget.Env{
				Settings: map[string]any{"calendars": []any{"c"}}, Connections: map[string]map[string]any{"c": {"url": u}}, Location: loc,
			}); err != nil {
				bad.Add(1)
				return
			}
			for i := 0; i < 5; i++ {
				v, err := p.Fetch(context.Background())
				if err != nil {
					bad.Add(1)
					return
				}
				// A floating 10:00 must be 10:00 in *this* provider's zone.
				for _, e := range v.(Data).Events {
					if h := time.Unix(e.Start, 0).In(loc).Hour(); h != 10 {
						bad.Add(1)
					}
				}
			}
		}()
	}
	wg.Wait()
	if bad.Load() != 0 {
		t.Errorf("%d inconsistent results under concurrency", bad.Load())
	}
}
