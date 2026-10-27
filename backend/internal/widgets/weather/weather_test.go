package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/widget"
)

// 2026-10-17 12:00:00 UTC; Berlin is UTC+2 on that day.
var now = time.Date(2026, 10, 17, 12, 30, 0, 0, time.UTC)

func hourly(n int) (times []int64, temps, prob, codes []any) {
	base := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC).Unix()
	for i := 0; i < n; i++ {
		times = append(times, base+int64(i)*3600)
		temps = append(temps, float64(i))
		prob = append(prob, 10)
		codes = append(codes, 61)
	}
	temps[30] = nil // null values must be skipped
	return
}

func upstream(t *testing.T, queries *[]string) *httptest.Server {
	t.Helper()
	times, temps, prob, codes := hourly(72)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*queries = append(*queries, r.URL.Path+"?"+r.URL.RawQuery)
		if strings.HasPrefix(r.URL.Path, "/geo") {
			json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"name": "Köln", "latitude": 50.93, "longitude": 6.95}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"timezone": "Europe/Berlin", "utc_offset_seconds": 7200,
			"current": map[string]any{"temperature_2m": 10.4, "apparent_temperature": 8.0, "relative_humidity_2m": 83.6,
				"precipitation": 0.2, "weather_code": 61, "wind_speed_10m": 12.3, "wind_direction_10m": 329.4, "is_day": 1},
			"hourly": map[string]any{"time": times, "temperature_2m": temps, "precipitation_probability": prob, "weather_code": codes},
			"daily": map[string]any{
				// Berlin local midnight of Oct 17 and 18 as true instants
				"time":               []int64{time.Date(2026, 10, 16, 22, 0, 0, 0, time.UTC).Unix(), time.Date(2026, 10, 17, 22, 0, 0, 0, time.UTC).Unix()},
				"weather_code":       []int{61, 3},
				"temperature_2m_max": []float64{13.6, 12}, "temperature_2m_min": []float64{8, 7},
				"precipitation_probability_max": []int{98, 20},
				"sunrise":                       []int64{1, 2}, "sunset": []int64{3, 4},
			},
		})
	}))
}

func start(t *testing.T, settings map[string]any, srv *httptest.Server) *Provider {
	t.Helper()
	p := newProvider()
	p.forecastURL = srv.URL + "/forecast"
	p.geocodingURL = srv.URL + "/geo"
	p.now = func() time.Time { return now }
	if err := p.Start(context.Background(), widget.Env{Settings: settings}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFetchMapsData(t *testing.T) {
	var q []string
	srv := upstream(t, &q)
	defer srv.Close()
	p := start(t, map[string]any{"latitude": 50.9, "longitude": 6.9, "label": "Zuhause"}, srv)

	raw, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d := raw.(Data)
	if d.Location != "Zuhause" || d.Timezone != "Europe/Berlin" {
		t.Errorf("location/timezone: %+v", d)
	}
	c := d.Current
	if c.Condition != ConditionRain || c.Humidity != 84 || c.WindDir != 329 || !c.IsDay {
		t.Errorf("current: %+v", c)
	}
	if len(d.Daily) != 2 || d.Daily[0].Date != "2026-10-17" || d.Daily[1].Date != "2026-10-18" || d.Daily[1].Condition != ConditionCloudy {
		t.Errorf("daily dates must be local days: %+v", d.Daily)
	}
	for _, key := range []string{"timezone=auto", "timeformat=unixtime", "latitude=50.9"} {
		if !strings.Contains(q[0], key) {
			t.Errorf("query lacks %s: %s", key, q[0])
		}
	}
}

func TestHourlyStartsAtCurrentHourAndSkipsNull(t *testing.T) {
	times, temps, prob, codes := hourly(72)
	var r response
	r.Timezone = "Europe/Berlin"
	r.Hourly.Time = times
	for i := range temps {
		r.Hourly.Temp = append(r.Hourly.Temp, f(temps[i]))
		r.Hourly.PrecipProb = append(r.Hourly.PrecipProb, f(prob[i]))
		r.Hourly.Code = append(r.Hourly.Code, f(codes[i]))
	}
	d, err := r.toData("x", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Hourly) != 24 {
		t.Fatalf("len = %d", len(d.Hourly))
	}
	if got := time.Unix(d.Hourly[0].T, 0).UTC().Hour(); got != 12 {
		t.Errorf("first hour = %d, want the hour containing now (12)", got)
	}
	for _, h := range d.Hourly {
		if h.T == times[30] {
			t.Error("null temperature must be skipped")
		}
	}
}

func f(v any) *float64 {
	if v == nil {
		return nil
	}
	var x float64
	switch n := v.(type) {
	case float64:
		x = n
	case int:
		x = float64(n)
	}
	return &x
}

func TestGeocodingIsCached(t *testing.T) {
	var q []string
	srv := upstream(t, &q)
	defer srv.Close()
	p := start(t, map[string]any{"location": "Köln"}, srv)

	for i := 0; i < 2; i++ {
		if _, err := p.Fetch(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	geo := 0
	for _, s := range q {
		if strings.HasPrefix(s, "/geo") {
			geo++
		}
	}
	if geo != 1 {
		t.Errorf("geocoding requests = %d, want 1", geo)
	}
	raw, _ := p.Fetch(context.Background())
	if raw.(Data).Location != "Köln" {
		t.Errorf("name should come from geocoding, got %q", raw.(Data).Location)
	}
}

func TestStartValidation(t *testing.T) {
	for name, s := range map[string]map[string]any{
		"nothing":      {},
		"only lat":     {"latitude": 50.0},
		"out of range": {"latitude": 123.0, "longitude": 0.0},
	} {
		if err := newProvider().Start(context.Background(), widget.Env{Settings: s}); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestUpstreamErrorIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	p := start(t, map[string]any{"latitude": 1.0, "longitude": 1.0}, srv)
	if _, err := p.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("want HTTP 429 error, got %v", err)
	}
}

func TestConditionMapping(t *testing.T) {
	for code, want := range map[int]Condition{0: ConditionClear, 2: ConditionPartlyCloudy, 45: ConditionFog, 53: ConditionDrizzle, 65: ConditionRain, 82: ConditionShowers, 75: ConditionSnow, 99: ConditionThunderstorm, 67: ConditionFreezingRain, 1234: ConditionCloudy} {
		if got := conditionFromWMO(code); got != want {
			t.Errorf("code %d = %s, want %s", code, got, want)
		}
	}
}
