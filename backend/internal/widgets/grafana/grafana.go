// Package grafana is the Grafana widget. Grafana is the single data gateway:
// the widget does not talk to Prometheus, InfluxDB or SQL itself, it asks
// Grafana's query API, so every datasource Grafana supports works. The widget
// either references an existing dashboard panel (its queries, unit, thresholds
// and range are taken from Grafana and stay in sync) or runs queries given in
// the config. Results are drawn natively as a chart, a stat or a gauge.
package grafana

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pmcgn/lumina/internal/httpx"
	"github.com/pmcgn/lumina/internal/widget"
)

const Type = "grafana.panel"

const (
	defaultInterval = 60 * time.Second
	minInterval     = 10 * time.Second
	defaultRange    = "6h"
	defaultPoints   = 300
	definitionTTL   = 10 * time.Minute // how long a panel definition is reused
)

func init() {
	widget.Register(Type, func() widget.Provider {
		return &Provider{client: &http.Client{Timeout: 30 * time.Second}, now: time.Now}
	})
}

type Provider struct {
	client *http.Client
	now    func() time.Time

	base   string
	header http.Header
	s      Settings

	interval  time.Duration
	maxPoints int

	mu        sync.Mutex
	def       *panelDef // cached panel definition (panel mode) or the settings-based one
	dsList    []dsInfo
	dsFetched time.Time
}

var rangeRe = regexp.MustCompile(`^[1-9][0-9]*[smhdw]$`)

// parseSettings decodes and validates the widget settings.
func parseSettings(m map[string]any) (Settings, error) {
	var s Settings
	if err := widget.DecodeSettings(m, &s); err != nil {
		return s, err
	}
	panelMode := s.Dashboard != "" || s.Panel != ""
	queryMode := s.Datasource != "" || len(s.Queries) > 0
	switch {
	case panelMode && queryMode:
		return s, errors.New("use either dashboard + panel, or datasource + queries, not both")
	case panelMode && s.Dashboard == "":
		return s, fmt.Errorf("setting %q is required together with %q", "dashboard", "panel")
	case panelMode && s.Panel == "":
		return s, fmt.Errorf("setting %q is required together with %q", "panel", "dashboard")
	case queryMode && s.Datasource == "":
		return s, fmt.Errorf("setting %q is required together with %q", "datasource", "queries")
	case queryMode && len(s.Queries) == 0:
		return s, fmt.Errorf("setting %q is required together with %q", "queries", "datasource")
	case !panelMode && !queryMode:
		return s, errors.New("set dashboard and panel (reuse a Grafana panel), or datasource and queries")
	}
	switch s.Display {
	case "", DisplayChart, DisplayStat, DisplayGauge:
	default:
		return s, fmt.Errorf("setting %q: %q is invalid (chart, stat, gauge)", "display", s.Display)
	}
	if s.Range != "" && !rangeRe.MatchString(s.Range) {
		return s, fmt.Errorf("setting %q: %q is invalid (examples: 30m, 6h, 7d)", "range", s.Range)
	}
	if s.Decimals != nil && (*s.Decimals < 0 || *s.Decimals > 10) {
		return s, fmt.Errorf("setting %q: must be 0..10", "decimals")
	}
	if s.MaxPoints != nil && (*s.MaxPoints < 50 || *s.MaxPoints > 1000) {
		return s, fmt.Errorf("setting %q: must be 50..1000", "maxPoints")
	}
	if s.Min != nil && s.Max != nil && *s.Min >= *s.Max {
		return s, errors.New("setting \"min\" must be smaller than \"max\"")
	}
	return s, nil
}

func (p *Provider) ValidateSettings(m map[string]any, _ map[string]map[string]any) error {
	_, err := parseSettings(m)
	return err
}

func (p *Provider) Start(_ context.Context, env widget.Env) error {
	c := env.Connection
	if c == nil {
		return errors.New("connection: this widget needs a connection with url and token")
	}
	raw := strings.TrimRight(widget.String(c, "url", ""), "/")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("connection.url: %q is not a valid http(s) URL", raw)
	}
	token := widget.String(c, "token", "")
	if token == "" {
		return errors.New("connection.token: missing (a Grafana service account token with the Viewer role)")
	}
	p.base = raw
	p.header = http.Header{"Authorization": {"Bearer " + token}}

	s, err := parseSettings(env.Settings)
	if err != nil {
		return err
	}
	p.s = s
	p.interval = defaultInterval
	if s.IntervalSeconds != nil {
		p.interval = time.Duration(*s.IntervalSeconds) * time.Second
	}
	if p.interval < minInterval {
		p.interval = minInterval
	}
	p.maxPoints = defaultPoints
	if s.MaxPoints != nil {
		p.maxPoints = *s.MaxPoints
	}
	return nil
}

func (p *Provider) Interval() time.Duration { return p.interval }

func (p *Provider) do(ctx context.Context, method, path string, in, out any) error {
	return httpx.DoJSON(ctx, p.client, method, p.base+path, p.header, in, out)
}

func (p *Provider) Fetch(ctx context.Context) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	def, err := p.definition(ctx)
	if err != nil {
		return nil, err
	}
	display := def.display
	if p.s.Display != "" {
		display = p.s.Display
	}
	if display == "" {
		return nil, fmt.Errorf("panel type %q is not supported: set display to chart, stat or gauge to draw it anyway", def.ptype)
	}

	from := def.from
	if p.s.Range != "" {
		from = "now-" + p.s.Range
	}
	if from == "" {
		from = "now-" + defaultRange
	}
	series, warnings, err := p.runQueries(ctx, def, from)
	if err != nil {
		p.def = nil // the panel may have changed in Grafana: read it again next time
		return nil, err
	}

	now := p.now()
	d := Data{
		Title:      def.title,
		Display:    display,
		Unit:       def.unit,
		Decimals:   def.decimals,
		Min:        def.min,
		Max:        def.max,
		Thresholds: def.thresholds,
		Series:     series,
		From:       now.Add(-6 * time.Hour).UnixMilli(),
		To:         now.UnixMilli(),
		Warnings:   warnings,
	}
	if t, ok := parseRelative(from, now); ok {
		d.From = t.UnixMilli()
	} else if first := earliest(series); first > 0 {
		d.From = first
	}
	if p.s.Title != "" {
		d.Title = p.s.Title
	}
	if p.s.Unit != "" {
		d.Unit = p.s.Unit
	}
	if p.s.Decimals != nil {
		d.Decimals = p.s.Decimals
	}
	if p.s.Min != nil {
		d.Min = p.s.Min
	}
	if p.s.Max != nil {
		d.Max = p.s.Max
	}
	if d.Series == nil {
		d.Series = []Series{}
	}
	return d, nil
}

// definition returns the panel to run: read from Grafana in panel mode (cached
// for a while), or built from the settings in query mode.
func (p *Provider) definition(ctx context.Context) (*panelDef, error) {
	if p.def != nil && p.now().Sub(p.def.fetched) < definitionTTL {
		return p.def, nil
	}
	if p.s.Dashboard == "" {
		qs := make([]map[string]any, len(p.s.Queries))
		for i, q := range p.s.Queries {
			qs[i] = q
		}
		p.def = &panelDef{
			title:      p.s.Title,
			display:    DisplayChart,
			targets:    qs,
			datasource: p.s.Datasource,
			fetched:    p.now(),
		}
		return p.def, nil
	}
	def, err := p.loadPanel(ctx)
	if err != nil {
		return nil, err
	}
	p.def = def
	return def, nil
}

func earliest(series []Series) int64 {
	var first int64
	for _, s := range series {
		if len(s.Points) > 0 && (first == 0 || s.Points[0].T < first) {
			first = s.Points[0].T
		}
	}
	return first
}
