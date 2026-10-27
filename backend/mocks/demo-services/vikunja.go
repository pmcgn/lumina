// Vikunja stand-in: the part of the API the dashboard uses (projects, open
// tasks, update, create), with tasks seeded relative to the current time.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const demoToken = "demo"

type vikunjaSim struct {
	mu     sync.Mutex
	tasks  map[int64]map[string]any
	nextID int64
}

var simProjects = []map[string]any{
	{"id": 1, "title": "Inbox"},
	{"id": 5, "title": "Smarthome"},
	{"id": 6, "title": "Haushalt"},
}

func newVikunja() *vikunjaSim {
	v := &vikunjaSim{tasks: map[int64]map[string]any{}, nextID: 100}
	now := time.Now()
	add := func(id int64, title string, project int, due *time.Time, prio int, repeat time.Duration, labels ...string) {
		var ls []map[string]any
		for _, l := range labels {
			ls = append(ls, map[string]any{"title": l, "hex_color": "10b981"})
		}
		d := "0001-01-01T00:00:00Z"
		if due != nil {
			d = due.Format(time.RFC3339)
		}
		v.tasks[id] = map[string]any{
			"id": id, "title": title, "description": "", "done": false, "due_date": d, "project_id": project,
			"priority": prio, "repeat_after": int64(repeat / time.Second), "repeat_mode": 0, "labels": ls,
		}
	}
	at := func(d time.Duration) *time.Time { t := now.Add(d); return &t }
	add(1, "Heizung entlüften", 5, at(-72*time.Hour), 4, 0)
	add(2, "Rauchmelder prüfen", 5, at(-2*time.Hour), 3, 0)
	add(3, "Müll rausbringen", 6, at(3*time.Hour), 0, 7*24*time.Hour) // recurring weekly
	add(4, "Router-Firmware updaten", 5, at(48*time.Hour), 0, 0)
	add(5, "Geschenk für Oma besorgen", 6, at(72*time.Hour), 0, 0, "Familie")
	add(6, "Nanoleaf-Panels aufhängen", 5, nil, 0, 0)
	add(7, "Steuererklärung vorbereiten", 1, at(20*24*time.Hour), 2, 0)
	add(8, "Shelly Küche tauschen", 5, nil, 0, 0)
	return v
}

func (v *vikunjaSim) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/", v.serve)
}

func (v *vikunjaSim) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+demoToken {
		http.Error(w, `{"message":"missing or invalid token"}`, http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	w.Header().Set("Content-Type", "application/json")
	v.mu.Lock()
	defer v.mu.Unlock()

	switch {
	case r.Method == http.MethodGet && path == "/projects":
		json.NewEncoder(w).Encode(simProjects)
	case r.Method == http.MethodGet && path == "/tasks":
		open := []map[string]any{}
		for id := int64(1); id < v.nextID; id++ {
			if t, ok := v.tasks[id]; ok && t["done"] == false {
				open = append(open, t)
			}
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page > 1 { // everything fits on the first page
			open = []map[string]any{}
		}
		json.NewEncoder(w).Encode(open)
	case strings.HasPrefix(path, "/tasks/") && r.Method == http.MethodGet:
		if t, ok := v.byPath(path); ok {
			json.NewEncoder(w).Encode(t)
			return
		}
		http.Error(w, `{"message":"task does not exist"}`, http.StatusNotFound)
	case strings.HasPrefix(path, "/tasks/") && r.Method == http.MethodPost:
		id, ok := parseID(path)
		if !ok || v.tasks[id] == nil {
			http.Error(w, `{"message":"task does not exist"}`, http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var t map[string]any
		if json.Unmarshal(body, &t) != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		// Like Vikunja: completing a recurring task moves its due date and keeps it open.
		if done, _ := t["done"].(bool); done {
			if every, _ := t["repeat_after"].(float64); every > 0 {
				due, err := time.Parse(time.RFC3339, fmt.Sprint(t["due_date"]))
				if err == nil && due.Year() > 1 {
					t["due_date"] = due.Add(time.Duration(every) * time.Second).Format(time.RFC3339)
					t["done"] = false
				}
			}
		}
		v.tasks[id] = t
		log.Printf("task %d: done=%v due=%v", id, t["done"], t["due_date"])
		json.NewEncoder(w).Encode(t)
	case strings.HasPrefix(path, "/projects/") && strings.HasSuffix(path, "/tasks") && r.Method == http.MethodPut:
		project, err := strconv.Atoi(strings.Split(strings.TrimPrefix(path, "/projects/"), "/")[0])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var in struct {
			Title string `json:"title"`
		}
		if json.Unmarshal(body, &in) != nil || strings.TrimSpace(in.Title) == "" {
			http.Error(w, `{"message":"title required"}`, http.StatusBadRequest)
			return
		}
		id := v.nextID
		v.nextID++
		v.tasks[id] = map[string]any{
			"id": id, "title": in.Title, "done": false, "due_date": "0001-01-01T00:00:00Z", "project_id": project,
			"priority": 0, "repeat_after": 0, "repeat_mode": 0, "labels": []any{},
		}
		log.Printf("task %d created: %s", id, in.Title)
		json.NewEncoder(w).Encode(v.tasks[id])
	default:
		http.NotFound(w, r)
	}
}

func (v *vikunjaSim) byPath(path string) (map[string]any, bool) {
	id, ok := parseID(path)
	if !ok {
		return nil, false
	}
	t, ok := v.tasks[id]
	return t, ok
}

func parseID(path string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimPrefix(path, "/tasks/"), 10, 64)
	return id, err == nil
}
