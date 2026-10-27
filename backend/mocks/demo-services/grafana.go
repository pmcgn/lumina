// Grafana stand-in: one dashboard ("Demo Haus") with a time series, a gauge and
// a stat panel, and a query endpoint that generates daily curves relative to
// the current time (consumption, solar, battery, temperature).
package main

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func registerGrafana(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboards/uid/", grafanaAuth(grafanaDashboard))
	mux.HandleFunc("/api/search", grafanaAuth(grafanaSearch))
	mux.HandleFunc("/api/datasources", grafanaAuth(grafanaDatasources))
	mux.HandleFunc("/api/ds/query", grafanaAuth(grafanaQuery))
}

// grafanaAuth wants a service account token like a real Grafana would.
func grafanaAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+demoToken {
			http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		next(w, r)
	}
}

var demoDatasource = map[string]any{"type": "influxdb", "uid": "demo-influx"}

func demoPanel(id int, typ, title string, targets []map[string]any, defaults map[string]any) map[string]any {
	return map[string]any{
		"id": id, "type": typ, "title": title, "datasource": demoDatasource,
		"targets": targets, "fieldConfig": map[string]any{"defaults": defaults},
	}
}

func grafanaDashboard(w http.ResponseWriter, r *http.Request) {
	if strings.TrimPrefix(r.URL.Path, "/api/dashboards/uid/") != "demo-haus" {
		http.Error(w, `{"message":"Dashboard not found"}`, http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"dashboard": map[string]any{
		"title": "Demo Haus",
		"time":  map[string]any{"from": "now-24h", "to": "now"},
		"panels": []any{
			demoPanel(1, "timeseries", "Stromverbrauch und Erzeugung", []map[string]any{
				{"refId": "A", "query": "consumption", "alias": "Hausverbrauch"},
				{"refId": "B", "query": "solar", "alias": "PV Erzeugung"},
			}, map[string]any{
				"unit":  "watt",
				"color": map[string]any{"mode": "palette-classic"},
				"thresholds": map[string]any{"steps": []any{
					map[string]any{"value": nil, "color": "green"}, map[string]any{"value": 80, "color": "red"}}},
			}),
			demoPanel(2, "gauge", "Batterie", []map[string]any{{"refId": "A", "query": "soc", "alias": "Ladezustand"}}, map[string]any{
				"unit": "percent", "min": 0, "max": 100,
				"thresholds": map[string]any{"steps": []any{
					map[string]any{"value": nil, "color": "dark-red"}, map[string]any{"value": 15, "color": "#EAB839"}, map[string]any{"value": 40, "color": "dark-green"}}},
			}),
			demoPanel(3, "stat", "Außentemperatur", []map[string]any{{"refId": "A", "query": "temp", "alias": "Garten"}}, map[string]any{
				"unit": "celsius", "decimals": 1,
				"thresholds": map[string]any{"steps": []any{
					map[string]any{"value": nil, "color": "blue"}, map[string]any{"value": 5, "color": "green"}, map[string]any{"value": 25, "color": "orange"}}},
			}),
		},
	}})
}

func grafanaSearch(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode([]map[string]any{{"uid": "demo-haus", "title": "Demo Haus", "type": "dash-db"}})
}

func grafanaDatasources(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode([]map[string]any{{"uid": "demo-influx", "name": "Demo InfluxDB", "type": "influxdb", "isDefault": true}})
}

var relRange = regexp.MustCompile(`^now-(\d+)([mhdw])$`)

func grafanaQuery(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var req struct {
		Queries []map[string]any `json:"queries"`
		From    string           `json:"from"`
	}
	if json.Unmarshal(body, &req) != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	now := time.Now()
	span := 6 * time.Hour
	if m := relRange.FindStringSubmatch(req.From); m != nil {
		n, _ := strconv.Atoi(m[1])
		span = time.Duration(n) * map[string]time.Duration{"m": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}[m[2]]
	}

	results := map[string]any{}
	for _, q := range req.Queries {
		ref, _ := q["refId"].(string)
		kind, _ := q["query"].(string)
		name, _ := q["alias"].(string)
		step := span / 240
		if step < time.Minute {
			step = time.Minute
		}
		var ts, vs []any
		for t := now.Add(-span).Truncate(step); !t.After(now); t = t.Add(step) {
			ts = append(ts, float64(t.UnixMilli()))
			vs = append(vs, math.Round(demoValue(kind, t)*10)/10)
		}
		results[ref] = map[string]any{"status": 200, "frames": []any{map[string]any{
			"schema": map[string]any{"fields": []any{
				map[string]any{"name": "Time", "type": "time"},
				map[string]any{"name": name, "type": "number", "config": map[string]any{"displayNameFromDS": name}},
			}},
			"data": map[string]any{"values": []any{ts, vs}},
		}}}
	}
	json.NewEncoder(w).Encode(map[string]any{"results": results})
}

// demoValue is a plausible household curve at time t.
func demoValue(kind string, t time.Time) float64 {
	h := float64(t.Hour()) + float64(t.Minute())/60
	switch kind {
	case "solar":
		return math.Max(0, 4200*math.Sin(math.Pi*(h-6.5)/13)) // 06:30-19:30
	case "consumption":
		morning := 900 * math.Exp(-math.Pow(h-7.5, 2)/2)
		evening := 1800 * math.Exp(-math.Pow(h-19, 2)/3)
		wobble := 80 * math.Sin(float64(t.Unix())/420)
		return 450 + morning + evening + wobble
	case "soc":
		return 55 + 40*math.Sin(math.Pi*(h-4)/12-math.Pi/2)*math.Cos(0)
	case "temp":
		return 9 + 6*math.Sin(2*math.Pi*(h-9)/24)
	}
	return 0
}
