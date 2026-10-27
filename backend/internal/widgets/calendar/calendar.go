// Package calendar is the calendar widget. It merges one or more iCal feeds
// (e.g. Google Calendar "secret address in iCal format"), expands recurring
// events, and delivers an agenda plus day markers for a month overview.
package calendar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pmcgn/lumina/internal/widget"
)

const Type = "calendar.agenda"

const (
	maxBody         = 10 << 20
	defaultDays     = 14
	maxDays         = 60
	defaultLimit    = 40
	defaultInterval = 10 * time.Minute
	gridWeeks       = 6
)

// defaultColors is used for calendars without a configured color.
var defaultColors = []string{"#3b82f6", "#f59e0b", "#10b981", "#ef4444", "#8b5cf6", "#ec4899", "#06b6d4", "#84cc16"}

func init() {
	widget.Register(Type, func() widget.Provider {
		return &Provider{client: &http.Client{Timeout: 25 * time.Second}, now: time.Now}
	})
}

type source struct {
	conn  string
	name  string
	color string
	url   string // https URL, or
	file  string // local path

	// cache of the last successful download
	body    []byte
	etag    string
	fetched time.Time
}

type Provider struct {
	client *http.Client
	now    func() time.Time

	mu        sync.Mutex
	sources   []*source
	days      int
	limit     int
	weekStart int
	interval  time.Duration
	loc       *time.Location
}

func (p *Provider) Start(_ context.Context, env widget.Env) error {
	p.loc = env.Location
	if p.loc == nil {
		p.loc = time.Local
	}
	s, sources, err := parseSettings(env.Settings, env.Connections)
	if err != nil {
		return err
	}
	p.sources = sources
	p.days = defaultDays
	if s.Days != nil {
		p.days = *s.Days
	}
	p.limit = defaultLimit
	if s.Limit != nil {
		p.limit = *s.Limit
	}
	p.interval = defaultInterval
	if s.IntervalMinutes != nil {
		p.interval = time.Duration(*s.IntervalMinutes) * time.Minute
	}
	if p.interval < time.Minute {
		p.interval = time.Minute
	}
	p.weekStart = 1
	if strings.ToLower(s.WeekStart) == "sunday" {
		p.weekStart = 0
	}
	return nil
}

// parseSettings decodes and validates the widget settings and resolves the
// calendar connections.
func parseSettings(m map[string]any, conns map[string]map[string]any) (Settings, []*source, error) {
	var s Settings
	if err := widget.DecodeSettings(m, &s); err != nil {
		return s, nil, err
	}
	if s.Days != nil && (*s.Days < 1 || *s.Days > maxDays) {
		return s, nil, fmt.Errorf("setting %q: must be 1..%d", "days", maxDays)
	}
	if s.Limit != nil && *s.Limit < 1 {
		return s, nil, fmt.Errorf("setting %q: must be >= 1", "limit")
	}
	switch strings.ToLower(s.WeekStart) {
	case "", "monday", "sunday":
	default:
		return s, nil, fmt.Errorf("setting %q: must be monday or sunday", "weekStart")
	}
	if len(s.Calendars) == 0 {
		return s, nil, fmt.Errorf("setting %q: list at least one calendar (connection names)", "calendars")
	}
	var sources []*source
	for i, ref := range s.Calendars {
		src, err := parseSource(ref, i, conns)
		if err != nil {
			return s, nil, fmt.Errorf("setting calendars[%d]: %w", i, err)
		}
		sources = append(sources, src)
	}
	return s, sources, nil
}

func (p *Provider) ValidateSettings(m map[string]any, conns map[string]map[string]any) error {
	_, _, err := parseSettings(m, conns)
	return err
}

// parseSource resolves one calendars entry to its feed.
func parseSource(ref CalendarRef, index int, conns map[string]map[string]any) (*source, error) {
	conn := ref.Connection
	if conn == "" {
		return nil, errors.New("connection name missing")
	}
	c, ok := conns[conn]
	if !ok {
		return nil, fmt.Errorf("unknown connection %q", conn)
	}
	src := &source{conn: conn, name: ref.Name, color: ref.Color}
	if src.name == "" {
		src.name = conn
	}
	if src.color == "" {
		src.color = defaultColors[index%len(defaultColors)]
	}
	src.url = strings.TrimSpace(widget.String(c, "url", ""))
	src.file = widget.String(c, "file", "")
	switch {
	case src.url != "" && src.file != "":
		return nil, fmt.Errorf("connection %q: set either url or file, not both", conn)
	case src.url != "":
		src.url = strings.Replace(src.url, "webcal://", "https://", 1)
		u, err := url.Parse(src.url)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			// the URL itself is a secret, so it must not appear in messages
			return nil, fmt.Errorf("connection %q: url is not a valid http(s) URL", conn)
		}
	case src.file == "":
		return nil, fmt.Errorf("connection %q: needs a url (iCal address) or a file", conn)
	}
	return src, nil
}

func (p *Provider) Interval() time.Duration { return p.interval }

func (p *Provider) Fetch(ctx context.Context) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now().In(p.loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, p.loc)
	agendaTo := today.AddDate(0, 0, p.days)
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, p.loc)
	gridFrom := first.AddDate(0, 0, -((int(first.Weekday()) - p.weekStart + 7) % 7))
	gridTo := gridFrom.AddDate(0, 0, gridWeeks*7)
	from, to := earlier(today, gridFrom), later(agendaTo, gridTo)

	d := Data{Timezone: p.loc.String(), WeekStart: p.weekStart, Events: []Event{}, Marks: map[string][]int{}, MonthFrom: gridFrom.Format("2006-01-02")}
	var all []Event
	usable := 0
	for i, src := range p.sources {
		info := Info{Name: src.name, Color: src.color}
		data, err := p.load(ctx, src)
		if err != nil {
			info.Error = err.Error()
		}
		if data != nil {
			evs, perr := expand(data, from, to, p.loc)
			if perr != nil {
				info.Error = perr.Error()
			} else {
				usable++
				for _, e := range evs {
					all = append(all, Event{Title: e.Title, Start: e.Start.Unix(), End: e.End.Unix(), AllDay: e.AllDay, Location: e.Location, Cal: i})
				}
			}
		}
		d.Calendars = append(d.Calendars, info)
	}
	if usable == 0 {
		return nil, fmt.Errorf("no calendar could be loaded: %s", firstError(d.Calendars))
	}

	sortEvents(all)
	for _, e := range all {
		if e.End > today.Unix() && e.Start < agendaTo.Unix() && len(d.Events) < p.limit {
			d.Events = append(d.Events, e)
		}
		markDays(d.Marks, e, p.loc, gridFrom, gridTo)
	}
	return d, nil
}

// load returns the calendar data, from the network (conditional request) or the
// cache. On failure the cached data is returned together with the error.
func (p *Provider) load(ctx context.Context, src *source) ([]byte, error) {
	if src.file != "" {
		b, err := os.ReadFile(src.file)
		if err != nil {
			return src.body, fmt.Errorf("cannot read calendar file: %w", errors.Unwrap(err))
		}
		src.body = b
		return b, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.url, nil)
	if err != nil {
		return src.body, errors.New("invalid calendar url")
	}
	req.Header.Set("User-Agent", "lumina")
	req.Header.Set("Accept", "text/calendar, */*")
	if src.etag != "" && src.body != nil {
		req.Header.Set("If-None-Match", src.etag)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		// net/http errors contain the full URL, which is a secret for private feeds
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return src.body, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified && src.body != nil:
		src.fetched = p.now()
		return src.body, nil
	case resp.StatusCode/100 != 2:
		return src.body, fmt.Errorf("calendar server answered HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return src.body, fmt.Errorf("download failed: %w", err)
	}
	if len(body) > maxBody {
		return src.body, errors.New("calendar is too large")
	}
	src.body, src.etag, src.fetched = body, resp.Header.Get("ETag"), p.now()
	return body, nil
}

func firstError(cs []Info) string {
	for _, c := range cs {
		if c.Error != "" {
			return c.Name + ": " + c.Error
		}
	}
	return "unknown error"
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func sortEvents(es []Event) {
	sort.SliceStable(es, func(i, j int) bool { return lessEvent(es[i], es[j]) })
}

func lessEvent(a, b Event) bool {
	if a.Start != b.Start {
		return a.Start < b.Start
	}
	if a.AllDay != b.AllDay {
		return a.AllDay
	}
	return a.Cal < b.Cal
}

// markDays records on which days of the month grid a calendar has events. An
// event marks every day it touches; the end is exclusive, so an event that ends
// at midnight does not mark the next day.
func markDays(marks map[string][]int, e Event, loc *time.Location, gridFrom, gridTo time.Time) {
	start := time.Unix(e.Start, 0).In(loc)
	end := time.Unix(e.End, 0).In(loc)
	if !end.After(start) {
		end = start.Add(time.Second) // zero-length events still mark their day
	}
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	for ; day.Before(end); day = day.AddDate(0, 0, 1) {
		if day.Before(gridFrom) || !day.Before(gridTo) {
			continue
		}
		key := day.Format("2006-01-02")
		if !contains(marks[key], e.Cal) {
			marks[key] = append(marks[key], e.Cal)
		}
	}
}

func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
