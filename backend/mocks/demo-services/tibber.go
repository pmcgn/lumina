package main

import (
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"time"
)

// registerTibber serves a stand-in for the Tibber Data API under /tibber/: the
// OAuth token endpoint (any refresh token is accepted) and the home and
// price endpoints with a generated day curve (cheap at night and midday, peaks
// in the morning and evening) for today and tomorrow, with the same paging
// (up to 192 slots, a "next" link) as the real API.
func registerTibber(mux *http.ServeMux) {
	mux.HandleFunc("POST /tibber/connect/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "demo", "expires_in": 3600, "refresh_token": "demo"})
	})
	mux.HandleFunc("GET /tibber/v1/homes", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"homes": []map[string]any{{"id": "demo-home", "name": "Zuhause"}}})
	})
	mux.HandleFunc("GET /tibber/v1/homes/{id}/prices", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		y, m, d := now.Date()
		midnight := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
		end := midnight.AddDate(0, 0, 1)
		if now.Hour() >= 13 {
			end = end.AddDate(0, 0, 1) // tomorrow is published in the afternoon
		}
		since := midnight
		if s, err := time.Parse(time.RFC3339, r.URL.Query().Get("since")); err == nil {
			since = s
		}
		items := []map[string]any{}
		for t := since; t.Before(end) && len(items) < 192; t = t.Add(15 * time.Minute) {
			items = append(items, map[string]any{
				"time":        t.Format(time.RFC3339),
				"consumption": map[string]any{"price": round4(demoPrice(t)), "vatPercent": 19},
			})
		}
		resp := map[string]any{"currency": "EUR", "items": items}
		if len(items) > 0 {
			next := end
			if len(items) == 192 {
				next = since.Add(192 * 15 * time.Minute)
			}
			resp["next"] = "/tibber/v1/homes/" + r.PathValue("id") + "/prices?since=" + url.QueryEscape(next.Format(time.RFC3339))
		}
		json.NewEncoder(w).Encode(resp)
	})
}

// demoPrice is EUR per kWh: a base of ~0.28 with a morning and an evening peak, a midday
// solar dip and a different level per weekday.
func demoPrice(t time.Time) float64 {
	h := float64(t.Hour()) + float64(t.Minute())/60
	peak := func(center, width, height float64) float64 { return height * math.Exp(-math.Pow((h-center)/width, 2)) }
	p := 0.26 + peak(7.5, 1.6, 0.09) + peak(18.5, 2, 0.14) - peak(13, 2.2, 0.1) - peak(3, 2, 0.04)
	return p + 0.01*float64(t.Weekday()%3)
}

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
