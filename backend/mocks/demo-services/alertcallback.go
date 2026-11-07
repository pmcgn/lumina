package main

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// registerAlertCallback is a target for the callbackUrl of an alert (docs/alerts.md):
// POST records a confirmation, GET lists them, so a test can see that the button worked.
func registerAlertCallback(mux *http.ServeMux) {
	var (
		mu   sync.Mutex
		seen []map[string]any
	)
	mux.HandleFunc("/alert-callback", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPost {
			var body map[string]any
			b, _ := io.ReadAll(io.LimitReader(r.Body, 4<<10))
			json.Unmarshal(b, &body)
			seen = append(seen, map[string]any{"receivedAt": time.Now().Format(time.RFC3339), "body": body})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		out := seen
		if out == nil {
			out = []map[string]any{}
		}
		json.NewEncoder(w).Encode(out)
	})
}
