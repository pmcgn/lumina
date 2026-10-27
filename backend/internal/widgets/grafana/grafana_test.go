package grafana

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/widget"
)

var now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// fakeGrafana serves the parts of Grafana's HTTP API the widget uses.
type fakeGrafana struct {
	mu         sync.Mutex
	dashboards map[string]map[string]any // uid -> dashboard JSON
	datasrc    []map[string]any
	frames     map[string][]map[string]any // refId -> frames
	errors     map[string]string           // refId -> error
	queryBody  map[string]any              // last /api/ds/query body
	queryCalls int
	dashCalls  int
	queryFails bool
	auth       string
}

func (f *fakeGrafana) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = r.Header.Get("Authorization")
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/dashboards/uid/"):
		f.dashCalls++
		uid := strings.TrimPrefix(r.URL.Path, "/api/dashboards/uid/")
		d, ok := f.dashboards[uid]
		if !ok {
			w.WriteHeader(404)
			io.WriteString(w, `{"message":"Dashboard not found"}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"dashboard": d})
	case r.URL.Path == "/api/search":
		var hits []map[string]any
		for uid, d := range f.dashboards {
			if strings.Contains(strings.ToLower(d["title"].(string)), strings.ToLower(r.URL.Query().Get("query"))) {
				hits = append(hits, map[string]any{"uid": uid, "title": d["title"]})
			}
		}
		json.NewEncoder(w).Encode(hits)
	case r.URL.Path == "/api/datasources":
		json.NewEncoder(w).Encode(f.datasrc)
	case r.URL.Path == "/api/ds/query" && r.Method == http.MethodPost:
		f.queryCalls++
		if f.queryFails {
			http.Error(w, `{"message":"boom"}`, 500)
			return
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &f.queryBody)
		results := map[string]any{}
		for _, q := range f.queryBody["queries"].([]any) {
			ref := q.(map[string]any)["refId"].(string)
			if msg, bad := f.errors[ref]; bad {
				results[ref] = map[string]any{"error": msg}
				continue
			}
			results[ref] = map[string]any{"status": 200, "frames": f.frames[ref]}
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	default:
		http.NotFound(w, r)
	}
}

func timeFrame(name string, ts []any, vs []any) map[string]any {
	return map[string]any{
		"schema": map[string]any{"fields": []any{
			map[string]any{"name": "Time", "type": "time"},
			map[string]any{"name": name, "type": "number", "config": map[string]any{}},
		}},
		"data": map[string]any{"values": []any{ts, vs}},
	}
}

func panel(id int, title, typ string, extra map[string]any) map[string]any {
	p := map[string]any{
		"id": id, "type": typ, "title": title,
		"datasource": map[string]any{"type": "influxdb", "uid": "ds-influx"},
		"targets":    []any{map[string]any{"refId": "A", "query": "from(bucket:\"x\")"}},
		"fieldConfig": map[string]any{"defaults": map[string]any{
			"unit": "watt", "decimals": 1.0, "min": 0.0, "max": 5000.0,
			"thresholds": map[string]any{"steps": []any{
				map[string]any{"value": nil, "color": "green"},
				map[string]any{"value": 3000.0, "color": "dark-red"},
				map[string]any{"value": 4000.0, "color": "#112233"},
				map[string]any{"value": 4500.0, "color": "javascript:alert(1)"},
			}},
		}},
	}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

func newFake(t *testing.T) (*fakeGrafana, string) {
	f := &fakeGrafana{
		dashboards: map[string]map[string]any{
			"pv-uid": {"title": "PV Übersicht", "time": map[string]any{"from": "now-6h", "to": "now"}, "panels": []any{
				panel(1, "Verbrauch", "timeseries", nil),
				map[string]any{"id": 9, "type": "row", "title": "Details", "panels": []any{panel(2, "Batterie", "gauge", nil)}},
				panel(3, "Tabelle", "table", nil),
			}},
		},
		datasrc: []map[string]any{
			{"uid": "ds-influx", "name": "InfluxDB", "type": "influxdb"},
			{"uid": "ds-mysql", "name": "MySQL", "type": "mysql", "isDefault": true},
		},
		frames: map[string][]map[string]any{
			"A": {timeFrame("Verbrauch", []any{1000.0, 2000.0, 3000.0, 4000.0}, []any{10.0, nil, 30.0, 40.0})},
		},
		errors: map[string]string{},
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func start(t *testing.T, url string, settings map[string]any) *Provider {
	t.Helper()
	p := &Provider{client: &http.Client{Timeout: 5 * time.Second}, now: func() time.Time { return now }}
	err := p.Start(context.Background(), widget.Env{
		Settings:   settings,
		Connection: map[string]any{"url": url + "/", "token": "glsa_secret_token"},
	})
	if err != nil {
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

func TestPanelByTitleReadsQueriesAndAppearance(t *testing.T) {
	f, url := newFake(t)
	p := start(t, url, map[string]any{"dashboard": "pv-uid", "panel": "verbrauch"}) // title, any case
	d := fetch(t, p)

	if d.Title != "Verbrauch" || d.Display != DisplayChart || d.Unit != "watt" || d.Decimals == nil || *d.Decimals != 1 {
		t.Errorf("appearance: %+v", d)
	}
	if d.Min == nil || *d.Min != 0 || d.Max == nil || *d.Max != 5000 {
		t.Errorf("min/max: %v %v", d.Min, d.Max)
	}
	if len(d.Thresholds) != 4 || d.Thresholds[0].Value != nil || d.Thresholds[0].Color != "#73BF69" || d.Thresholds[1].Color != "#C4162A" || d.Thresholds[2].Color != "#112233" {
		t.Errorf("thresholds: %+v", d.Thresholds)
	}
	if d.Thresholds[3].Color != "" {
		t.Errorf("an unsafe color must be dropped, got %q", d.Thresholds[3].Color)
	}
	if len(d.Series) != 1 || d.Series[0].Name != "Verbrauch" {
		t.Fatalf("series: %+v", d.Series)
	}
	if got := d.Series[0].Points; len(got) != 3 || got[0] != (Point{1000, 10}) || got[2] != (Point{4000, 40}) {
		t.Errorf("null values must be skipped: %+v", got)
	}
	if f.auth != "Bearer glsa_secret_token" {
		t.Errorf("auth header = %q", f.auth)
	}

	// The panel's own query is re-run with the resolved datasource and the dashboard's range.
	if f.queryBody["from"] != "now-6h" || f.queryBody["to"] != "now" {
		t.Errorf("range: %v - %v", f.queryBody["from"], f.queryBody["to"])
	}
	q := f.queryBody["queries"].([]any)[0].(map[string]any)
	ds := q["datasource"].(map[string]any)
	if q["refId"] != "A" || ds["uid"] != "ds-influx" || ds["type"] != "influxdb" || q["query"] != `from(bucket:"x")` || q["maxDataPoints"].(float64) != 300 {
		t.Errorf("query: %+v", q)
	}
	if want := now.Add(-6 * time.Hour).UnixMilli(); d.From != want || d.To != now.UnixMilli() {
		t.Errorf("range ms: %d..%d", d.From, d.To)
	}
}

func TestPanelByIdInsideARowAndDashboardByTitle(t *testing.T) {
	f, url := newFake(t)
	f.frames["A"] = []map[string]any{timeFrame("Batterie", []any{1000.0}, []any{83.0})}
	p := start(t, url, map[string]any{"dashboard": "PV Übersicht", "panel": 2}) // title lookup, panel inside a row, by id
	d := fetch(t, p)
	if d.Title != "Batterie" || d.Display != DisplayGauge {
		t.Errorf("%+v", d)
	}
}

func TestRangeOverrideAndSettingOverrides(t *testing.T) {
	f, url := newFake(t)
	p := start(t, url, map[string]any{"dashboard": "pv-uid", "panel": 1, "range": "24h", "title": "Mein Titel", "unit": "kwatth", "display": "stat", "min": 1, "max": 9})
	d := fetch(t, p)
	if f.queryBody["from"] != "now-24h" {
		t.Errorf("range = %v", f.queryBody["from"])
	}
	if d.Title != "Mein Titel" || d.Unit != "kwatth" || d.Display != DisplayStat || *d.Min != 1 || *d.Max != 9 {
		t.Errorf("overrides: %+v", d)
	}
	if want := now.Add(-24 * time.Hour).UnixMilli(); d.From != want {
		t.Errorf("from = %d, want %d", d.From, want)
	}
}

func TestErrorsAreHelpful(t *testing.T) {
	_, url := newFake(t)
	for name, c := range map[string]struct {
		settings map[string]any
		want     string
	}{
		"unknown dashboard": {map[string]any{"dashboard": "gibt-es-nicht", "panel": 1}, "not found"},
		"unknown panel":     {map[string]any{"dashboard": "pv-uid", "panel": "Nope"}, `"Verbrauch" (id 1)`},
		"unsupported type":  {map[string]any{"dashboard": "pv-uid", "panel": "Tabelle"}, "not supported"},
	} {
		t.Run(name, func(t *testing.T) {
			p := start(t, url, c.settings)
			_, err := p.Fetch(context.Background())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
			if strings.Contains(err.Error(), "glsa_secret_token") {
				t.Error("the token must never appear in an error")
			}
		})
	}

	// A table panel can be drawn when the display is chosen.
	p := start(t, url, map[string]any{"dashboard": "pv-uid", "panel": "Tabelle", "display": "stat"})
	if d := fetch(t, p); d.Display != DisplayStat {
		t.Errorf("display override: %v", d.Display)
	}
}

func TestDatasourceResolution(t *testing.T) {
	f, url := newFake(t)
	dash := f.dashboards["pv-uid"]
	panels := dash["panels"].([]any)

	cases := map[string]struct {
		ds      any
		wantUID string
		wantErr string
	}{
		"legacy name":    {"MySQL", "ds-mysql", ""},
		"default (null)": {nil, "ds-mysql", ""},
		"object by uid":  {map[string]any{"type": "influxdb", "uid": "ds-influx"}, "ds-influx", ""},
		"dashboard var":  {map[string]any{"type": "influxdb", "uid": "${DS}"}, "", "dashboard variable"},
		"unknown":        {map[string]any{"uid": "zzz"}, "", "not found in Grafana"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			panels[0] = panel(1, "Verbrauch", "timeseries", map[string]any{"datasource": c.ds})
			dash["panels"] = panels
			p := start(t, url, map[string]any{"dashboard": "pv-uid", "panel": 1})
			_, err := p.Fetch(context.Background())
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want %q, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := f.queryBody["queries"].([]any)[0].(map[string]any)["datasource"].(map[string]any)["uid"]
			if got != c.wantUID {
				t.Errorf("datasource = %v, want %v", got, c.wantUID)
			}
		})
	}
}

func TestQueryMode(t *testing.T) {
	f, url := newFake(t)
	f.frames["A"] = []map[string]any{timeFrame("up", []any{1000.0, 2000.0}, []any{1.0, 0.0})}
	f.frames["B"] = []map[string]any{timeFrame("mem", []any{1000.0, 2000.0}, []any{5.0, 6.0})}
	p := start(t, url, map[string]any{
		"datasource": "InfluxDB", "title": "Server", "unit": "percent",
		"queries": []any{map[string]any{"query": "q1"}, map[string]any{"refId": "B", "query": "q2", "hide": false}, map[string]any{"query": "hidden", "hide": true}},
	})
	d := fetch(t, p)
	qs := f.queryBody["queries"].([]any)
	if len(qs) != 2 {
		t.Fatalf("hidden queries must be skipped: %d", len(qs))
	}
	if qs[0].(map[string]any)["refId"] != "A" || qs[1].(map[string]any)["refId"] != "B" {
		t.Errorf("refIds: %v", qs)
	}
	if d.Title != "Server" || d.Unit != "percent" || d.Display != DisplayChart || len(d.Series) != 2 || d.Series[0].Name != "up" || d.Series[1].Name != "mem" {
		t.Errorf("%+v", d)
	}
	if f.dashCalls != 0 {
		t.Error("query mode must not read dashboards")
	}
}

func TestPartialAndTotalQueryFailure(t *testing.T) {
	f, url := newFake(t)
	dash := f.dashboards["pv-uid"]
	dash["panels"] = []any{panel(1, "Zwei", "timeseries", map[string]any{"targets": []any{
		map[string]any{"refId": "A", "query": "ok"}, map[string]any{"refId": "B", "query": "kaputt"},
	}})}
	f.frames["A"] = []map[string]any{timeFrame("ok", []any{1000.0}, []any{1.0})}
	f.errors["B"] = "bucket not found:\nfoo"
	p := start(t, url, map[string]any{"dashboard": "pv-uid", "panel": 1})

	d := fetch(t, p)
	if len(d.Series) != 1 || len(d.Warnings) != 1 || !strings.Contains(d.Warnings[0], "query B: bucket not found: foo") {
		t.Errorf("partial failure must keep the working query and warn: %+v", d)
	}

	f.errors["A"] = "boom"
	if _, err := p.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "query A") {
		t.Errorf("all queries failing is an error: %v", err)
	}
}

func TestTableFrameYieldsTheLastValue(t *testing.T) {
	f, url := newFake(t)
	f.frames["A"] = []map[string]any{{
		"schema": map[string]any{"fields": []any{map[string]any{"name": "soc", "type": "number"}, map[string]any{"name": "text", "type": "string"}}},
		"data":   map[string]any{"values": []any{[]any{50.0, 83.0, nil}, []any{"a", "b", "c"}}},
	}}
	d := fetch(t, start(t, url, map[string]any{"dashboard": "pv-uid", "panel": 1, "display": "gauge"}))
	if len(d.Series) != 1 || len(d.Series[0].Points) != 1 || d.Series[0].Points[0].V != 83 || d.Series[0].Name != "soc" {
		t.Errorf("last non-null value: %+v", d.Series)
	}
}

func TestSeriesNamesAndColors(t *testing.T) {
	f, url := newFake(t)
	dash := f.dashboards["pv-uid"]
	dash["panels"] = []any{panel(1, "P", "timeseries", map[string]any{
		"fieldConfig": map[string]any{
			"defaults": map[string]any{"color": map[string]any{"mode": "fixed", "fixedColor": "blue"}},
			"overrides": []any{map[string]any{
				"matcher":    map[string]any{"id": "byName", "options": "Erzeugung"},
				"properties": []any{map[string]any{"id": "color", "value": map[string]any{"mode": "fixed", "fixedColor": "orange"}}},
			}},
		},
	})}
	labeled := timeFrame("Value", []any{1000.0}, []any{1.0})
	labeled["schema"].(map[string]any)["fields"].([]any)[1].(map[string]any)["labels"] = map[string]any{"node": "n1", "app": "x"}
	named := timeFrame("Erzeugung", []any{1000.0}, []any{2.0})
	displayName := timeFrame("raw", []any{1000.0}, []any{3.0})
	displayName["schema"].(map[string]any)["fields"].([]any)[1].(map[string]any)["config"] = map[string]any{"displayNameFromDS": "Schöner Name"}
	f.frames["A"] = []map[string]any{labeled, named, displayName}

	d := fetch(t, start(t, url, map[string]any{"dashboard": "pv-uid", "panel": 1}))
	if len(d.Series) != 3 {
		t.Fatalf("%+v", d.Series)
	}
	if d.Series[0].Name != "Value{app=x, node=n1}" {
		t.Errorf("labels must be part of the name, sorted: %q", d.Series[0].Name)
	}
	if d.Series[1].Name != "Erzeugung" || d.Series[1].Color != "#FF9830" {
		t.Errorf("byName override color: %+v", d.Series[1])
	}
	if d.Series[2].Name != "Schöner Name" || d.Series[2].Color != "#5794F2" {
		t.Errorf("displayNameFromDS and the fixed panel color: %+v", d.Series[2])
	}
}

func TestDownsamplingKeepsTheNewestPoint(t *testing.T) {
	f, url := newFake(t)
	var ts, vs []any
	for i := 0; i < 5000; i++ {
		ts = append(ts, float64(1000+i*60000))
		vs = append(vs, float64(i%97))
	}
	f.frames["A"] = []map[string]any{timeFrame("v", ts, vs)}
	d := fetch(t, start(t, url, map[string]any{"dashboard": "pv-uid", "panel": 1, "maxPoints": 120}))
	pts := d.Series[0].Points
	if len(pts) != 120 || pts[len(pts)-1].T != int64(1000+4999*60000) || pts[0].T != 1000 {
		t.Errorf("%d points, first %v last %v", len(pts), pts[0], pts[len(pts)-1])
	}
}

func TestPanelDefinitionIsCachedAndRefreshedAfterAFailure(t *testing.T) {
	f, url := newFake(t)
	p := start(t, url, map[string]any{"dashboard": "pv-uid", "panel": 1})
	fetch(t, p)
	fetch(t, p)
	if f.dashCalls != 1 || f.queryCalls != 2 {
		t.Errorf("the panel definition should be reused: %d dashboard reads, %d queries", f.dashCalls, f.queryCalls)
	}
	f.queryFails = true
	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	f.queryFails = false
	fetch(t, p)
	if f.dashCalls != 2 {
		t.Errorf("after a failure the panel is read again (it may have changed): %d reads", f.dashCalls)
	}
}

func TestStartValidation(t *testing.T) {
	conn := map[string]any{"url": "https://g.example", "token": "t"}
	good := map[string]any{"dashboard": "d", "panel": 1}
	for name, env := range map[string]widget.Env{
		"no connection": {Settings: good},
		"bad url":       {Connection: map[string]any{"url": "nope", "token": "t"}, Settings: good},
		"no token":      {Connection: map[string]any{"url": "https://g.example"}, Settings: good},
		"no settings":   {Connection: conn, Settings: map[string]any{}},
	} {
		if err := (&Provider{}).Start(context.Background(), env); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseRelative(t *testing.T) {
	for in, want := range map[string]time.Duration{"now-6h": 6 * time.Hour, "now-30m": 30 * time.Minute, "now-7d": 7 * 24 * time.Hour, "now-1w/w": 7 * 24 * time.Hour} {
		got, ok := parseRelative(in, now)
		if !ok || now.Sub(got) != want {
			t.Errorf("%s -> %v %v", in, got, ok)
		}
	}
	for _, in := range []string{"now", "now/d", "2026-01-01T00:00:00Z", "1700000000000", ""} {
		if _, ok := parseRelative(in, now); ok {
			t.Errorf("%q is not a relative range start we can compute", in)
		}
	}
}

func TestColorNormalization(t *testing.T) {
	for in, want := range map[string]string{
		"green": "#73BF69", "Dark-Red": "#C4162A", "#abc": "#abc", "#112233": "#112233", "rgb(10, 20, 30)": "rgb(10, 20, 30)",
		"rgba(0,0,0,0.5)": "rgba(0,0,0,0.5)", "": "", "javascript:alert(1)": "", "red;background:url(x)": "", "url(x)": "", "nonsense": "",
	} {
		if got := normalizeColor(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}
