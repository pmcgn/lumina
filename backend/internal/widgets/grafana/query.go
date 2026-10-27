package grafana

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pmcgn/lumina/internal/history"
)

// queryResp is the part of Grafana's /api/ds/query response we use.
type queryResp struct {
	Results map[string]struct {
		Error  string  `json:"error"`
		Frames []frame `json:"frames"`
	} `json:"results"`
}

type frame struct {
	Schema struct {
		Name   string  `json:"name"`
		Fields []field `json:"fields"`
	} `json:"schema"`
	Data struct {
		Values [][]any `json:"values"`
	} `json:"data"`
}

type field struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels"`
	Config struct {
		DisplayNameFromDS string `json:"displayNameFromDS"`
		DisplayName       string `json:"displayName"`
	} `json:"config"`
}

// runQueries executes the queries through Grafana (which resolves macros such
// as $__timeFilter and Flux's v.timeRangeStart itself) and converts the result
// frames to series.
func (p *Provider) runQueries(ctx context.Context, def *panelDef, from string) ([]Series, []string, error) {
	queries, err := p.buildQueries(ctx, def)
	if err != nil {
		return nil, nil, err
	}
	body := map[string]any{"queries": queries, "from": from, "to": "now"}
	var resp queryResp
	if err := p.do(ctx, http.MethodPost, "/api/ds/query", body, &resp); err != nil {
		return nil, nil, err
	}

	refs := make([]string, 0, len(resp.Results))
	for ref := range resp.Results {
		refs = append(refs, ref)
	}
	sort.Strings(refs) // map order is random; keep series stable

	var series []Series
	var warnings []string
	for _, ref := range refs {
		r := resp.Results[ref]
		if r.Error != "" {
			warnings = append(warnings, fmt.Sprintf("query %s: %s", ref, shorten(r.Error, 160)))
			continue
		}
		for _, f := range r.Frames {
			series = append(series, framesToSeries(f, p.now())...)
		}
	}
	for i := range series {
		series[i].Points = downsample(series[i].Points, p.maxPoints)
		if c := def.colorByName[series[i].Name]; c != "" {
			series[i].Color = c
		} else if def.fixedColor != "" {
			series[i].Color = def.fixedColor
		}
	}
	if len(series) == 0 && len(warnings) > 0 {
		return nil, nil, fmt.Errorf("%s", strings.Join(warnings, "; "))
	}
	return series, warnings, nil
}

// buildQueries prepares Grafana query objects: every query gets its datasource,
// a refId, and the interval/point limits.
func (p *Provider) buildQueries(ctx context.Context, def *panelDef) ([]map[string]any, error) {
	var out []map[string]any
	for i, t := range def.targets {
		if hidden, _ := t["hide"].(bool); hidden {
			continue
		}
		q := make(map[string]any, len(t)+3)
		for k, v := range t {
			q[k] = v
		}
		ref := def.datasource
		if own, ok := t["datasource"]; ok && own != nil {
			ref = own
		}
		if m, ok := ref.(map[string]any); ok && m["uid"] == "-- Mixed --" {
			return nil, fmt.Errorf("query %d of a mixed-datasource panel has no datasource of its own", i+1)
		}
		ds, err := p.resolveDatasource(ctx, ref)
		if err != nil {
			return nil, err
		}
		q["datasource"] = map[string]any{"uid": ds.UID, "type": ds.Type}
		if id, _ := q["refId"].(string); id == "" {
			q["refId"] = refLetter(i)
		}
		q["maxDataPoints"] = p.maxPoints
		q["intervalMs"] = 60000
		out = append(out, q)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the panel has no (visible) queries")
	}
	return out, nil
}

func refLetter(i int) string {
	if i < 26 {
		return string(rune('A' + i))
	}
	return "Q" + strconv.Itoa(i)
}

// framesToSeries converts one data frame: every numeric field becomes a series
// over the frame's time field. A frame without a time field (a table) yields
// the last value of each numeric field, stamped with the current time.
func framesToSeries(f frame, now time.Time) []Series {
	vals := f.Data.Values
	timeIdx := -1
	for i, fl := range f.Schema.Fields {
		if fl.Type == "time" {
			timeIdx = i
			break
		}
	}
	var out []Series
	for i, fl := range f.Schema.Fields {
		if fl.Type != "number" || i >= len(vals) {
			continue
		}
		s := Series{Name: seriesName(fl, f.Schema.Name)}
		if timeIdx >= 0 && timeIdx < len(vals) {
			ts := vals[timeIdx]
			for j, raw := range vals[i] {
				v, ok := toFloat(raw)
				if !ok || j >= len(ts) {
					continue
				}
				t, ok := toFloat(ts[j])
				if !ok {
					continue
				}
				s.Points = append(s.Points, Point{T: int64(t), V: v})
			}
			sort.SliceStable(s.Points, func(a, b int) bool { return s.Points[a].T < s.Points[b].T })
		} else {
			for j := len(vals[i]) - 1; j >= 0; j-- {
				if v, ok := toFloat(vals[i][j]); ok {
					s.Points = []Point{{T: now.UnixMilli(), V: v}}
					break
				}
			}
		}
		if len(s.Points) > 0 {
			out = append(out, s)
		}
	}
	return out
}

func seriesName(fl field, frameName string) string {
	name := fl.Config.DisplayNameFromDS
	if name == "" {
		name = fl.Config.DisplayName
	}
	if name == "" {
		name = fl.Name
		if (name == "Value" || name == "value") && frameName != "" {
			name = frameName
		}
		if len(fl.Labels) > 0 {
			keys := make([]string, 0, len(fl.Labels))
			for k := range fl.Labels {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, len(keys))
			for i, k := range keys {
				parts[i] = k + "=" + fl.Labels[k]
			}
			name += "{" + strings.Join(parts, ", ") + "}"
		}
	}
	return name
}

func toFloat(v any) (float64, bool) {
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// downsample keeps the visual shape of a series with at most n points (LTTB).
func downsample(pts []Point, n int) []Point {
	in := make([]history.Point, len(pts))
	for i, p := range pts {
		in[i] = history.Point{T: p.T, V: p.V}
	}
	red := history.Downsample(in, n)
	if len(red) == len(pts) {
		return pts
	}
	out := make([]Point, len(red))
	for i, p := range red {
		out[i] = Point{T: p.T, V: p.V}
	}
	return out
}

func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

var relativeFrom = regexp.MustCompile(`^now-(\d+)([smhdw])(?:/[smhdwMy])?$`)

// parseRelative reads Grafana's "now-6h" style range starts.
func parseRelative(expr string, now time.Time) (time.Time, bool) {
	m := relativeFrom.FindStringSubmatch(expr)
	if m == nil {
		return time.Time{}, false
	}
	n, _ := strconv.Atoi(m[1])
	unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}[m[2]]
	return now.Add(-time.Duration(n) * unit), true
}
