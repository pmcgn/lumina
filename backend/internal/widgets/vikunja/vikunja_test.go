package vikunja

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/widget"
)

var (
	berlin = mustLoc("Europe/Berlin")
	// Saturday 2026-10-17 12:00 Berlin
	now = time.Date(2026, 10, 17, 12, 0, 0, 0, berlin)
)

func mustLoc(n string) *time.Location {
	l, err := time.LoadLocation(n)
	if err != nil {
		panic(err)
	}
	return l
}

// fake is a minimal Vikunja API.
type fake struct {
	mu       sync.Mutex
	tasks    map[int64]map[string]any
	projects []map[string]any
	posted   []map[string]any // bodies of POST /tasks/{id}
	created  []map[string]any
	auth     string
}

func at(d time.Time) string { return d.UTC().Format(time.RFC3339) }

func newFake() *fake {
	zero := "0001-01-01T00:00:00Z"
	f := &fake{tasks: map[int64]map[string]any{}}
	f.projects = []map[string]any{{"id": 1, "title": "Inbox"}, {"id": 5, "title": "Smarthome"}}
	add := func(id int64, title string, project int, due string, prio int, labels ...string) {
		var ls []map[string]any
		for _, l := range labels {
			ls = append(ls, map[string]any{"title": l, "hex_color": "ff0000"})
		}
		f.tasks[id] = map[string]any{"id": id, "title": title, "done": false, "due_date": due, "project_id": project,
			"priority": prio, "labels": ls, "description": "keep me", "repeat_after": 0, "repeat_mode": 0}
	}
	add(1, "no date", 5, zero, 0)
	add(2, "tomorrow", 5, at(now.Add(24*time.Hour)), 0)
	add(3, "overdue 3d", 5, at(now.Add(-72*time.Hour)), 0, "home")
	add(4, "overdue 1h", 1, at(now.Add(-time.Hour)), 3)
	add(5, "later today", 5, at(now.Add(4*time.Hour)), 0, "home", "urgent")
	add(6, "next month", 5, at(now.AddDate(0, 1, 0)), 0)
	return f
}

func (f *fake) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = r.Header.Get("Authorization")
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && path == "/projects":
			json.NewEncoder(w).Encode(f.projects)
		case r.Method == "GET" && path == "/tasks":
			if !strings.Contains(r.URL.Query().Get("filter"), "done = false") {
				t.Errorf("filter must restrict to open tasks: %q", r.URL.Query().Get("filter"))
			}
			var out []map[string]any
			for id := int64(1); id <= int64(len(f.tasks)); id++ {
				if d, _ := f.tasks[id]["done"].(bool); !d {
					out = append(out, f.tasks[id])
				}
			}
			// emulate pagination
			page := 1
			fmt.Sscan(r.URL.Query().Get("page"), &page)
			lo, hi := (page-1)*50, page*50
			if lo > len(out) {
				lo = len(out)
			}
			if hi > len(out) {
				hi = len(out)
			}
			json.NewEncoder(w).Encode(out[lo:hi])
		case strings.HasPrefix(path, "/tasks/") && r.Method == "GET":
			var id int64
			fmt.Sscan(strings.TrimPrefix(path, "/tasks/"), &id)
			if tk, ok := f.tasks[id]; ok {
				json.NewEncoder(w).Encode(tk)
				return
			}
			http.Error(w, `{"message":"not found"}`, 404)
		case strings.HasPrefix(path, "/tasks/") && r.Method == "POST":
			var id int64
			fmt.Sscan(strings.TrimPrefix(path, "/tasks/"), &id)
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			json.Unmarshal(body, &m)
			f.posted = append(f.posted, m)
			f.tasks[id] = m
			json.NewEncoder(w).Encode(m)
		case strings.HasSuffix(path, "/tasks") && r.Method == "PUT":
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			json.Unmarshal(body, &m)
			f.created = append(f.created, map[string]any{"path": path, "body": m})
			json.NewEncoder(w).Encode(map[string]any{"id": 99, "title": m["title"]})
		default:
			http.NotFound(w, r)
		}
	})
}

func start(t *testing.T, f *fake, settings map[string]any) *Provider {
	t.Helper()
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	p := &Provider{client: srv.Client(), now: func() time.Time { return now }}
	err := p.Start(context.Background(), widget.Env{
		Settings:   settings,
		Connection: map[string]any{"url": srv.URL + "/", "token": "tk_secret"},
		Location:   berlin,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func titles(d Data) string {
	var s []string
	for _, t := range d.Tasks {
		s = append(s, t.Title)
	}
	return strings.Join(s, " | ")
}

func fetch(t *testing.T, p *Provider) Data {
	t.Helper()
	v, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return v.(Data)
}

func TestListsOpenTasksOverdueFirst(t *testing.T) {
	f := newFake()
	p := start(t, f, map[string]any{})
	d := fetch(t, p)

	want := "overdue 3d | overdue 1h | later today | tomorrow | next month | no date"
	if got := titles(d); got != want {
		t.Errorf("order:\n got %s\nwant %s", got, want)
	}
	if f.auth != "Bearer tk_secret" {
		t.Errorf("auth header = %q", f.auth)
	}
	if d.Tasks[0].Project != "Smarthome" || d.Tasks[1].Project != "Inbox" {
		t.Errorf("project names: %+v", d.Tasks[:2])
	}
	if d.Tasks[5].Due != 0 {
		t.Errorf("task without due date must have no due: %+v", d.Tasks[5])
	}
	if d.Tasks[2].Labels[1] != (Label{Title: "urgent", Color: "ff0000"}) {
		t.Errorf("labels: %+v", d.Tasks[2].Labels)
	}
}

func TestFilters(t *testing.T) {
	for name, tc := range map[string]struct {
		settings map[string]any
		want     string
	}{
		"overdue": {map[string]any{"due": "overdue"}, "overdue 3d | overdue 1h"},
		"today":   {map[string]any{"due": "today"}, "overdue 3d | overdue 1h | later today"},
		"week":    {map[string]any{"due": "week"}, "overdue 3d | overdue 1h | later today | tomorrow"},
		"project": {map[string]any{"projects": []any{"inbox"}}, "overdue 1h"},
		"proj id": {map[string]any{"projects": []any{5}, "due": "today"}, "overdue 3d | later today"},
		"labels":  {map[string]any{"labels": []any{"HOME", "urgent"}}, "later today"},
		"limit":   {map[string]any{"limit": 2}, "overdue 3d | overdue 1h"},
	} {
		t.Run(name, func(t *testing.T) {
			d := fetch(t, start(t, newFake(), tc.settings))
			if got := titles(d); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestLimitKeepsTotal(t *testing.T) {
	d := fetch(t, start(t, newFake(), map[string]any{"limit": 2}))
	if len(d.Tasks) != 2 || d.Total != 6 {
		t.Errorf("tasks=%d total=%d", len(d.Tasks), d.Total)
	}
}

func TestTodayUsesConfiguredTimezone(t *testing.T) {
	// 00:30 Berlin on Oct 18 is 22:30 UTC on Oct 17: "tomorrow" in Berlin, still "today" in UTC.
	due := at(time.Date(2026, 10, 18, 0, 30, 0, 0, berlin))
	for name, tc := range map[string]struct {
		loc  *time.Location
		want bool
	}{"berlin": {berlin, false}, "utc": {time.UTC, true}} {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			f.tasks[2]["due_date"] = due
			p := start(t, f, map[string]any{"due": "today"})
			p.loc = tc.loc
			got := strings.Contains(titles(fetch(t, p)), "tomorrow") // task 2
			if got != tc.want {
				t.Errorf("task in 'today' window = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPagination(t *testing.T) {
	f := newFake()
	for id := int64(7); id <= 130; id++ {
		f.tasks[id] = map[string]any{"id": id, "title": fmt.Sprint("t", id), "done": false, "due_date": "0001-01-01T00:00:00Z", "project_id": 5}
	}
	d := fetch(t, start(t, f, map[string]any{"limit": 500}))
	if d.Total != 130 {
		t.Errorf("total = %d, want 130 (3 pages)", d.Total)
	}
}

func TestCompleteReadsThenWritesFullTask(t *testing.T) {
	f := newFake()
	p := start(t, f, map[string]any{})
	res, err := p.Action(context.Background(), "complete", json.RawMessage(`{"id":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.(doneResult).Done {
		t.Errorf("result: %+v", res)
	}
	if len(f.posted) != 1 {
		t.Fatalf("posts = %d", len(f.posted))
	}
	body := f.posted[0]
	if body["done"] != true || body["description"] != "keep me" || body["title"] != "overdue 3d" || body["project_id"] == nil {
		t.Errorf("the whole task must be written back with done=true: %+v", body)
	}
	if got := titles(fetch(t, p)); strings.Contains(got, "overdue 3d") {
		t.Errorf("completed task still listed: %s", got)
	}
}

func TestUncompleteRestoresDueDate(t *testing.T) {
	f := newFake()
	p := start(t, f, map[string]any{})
	old := now.Add(-72 * time.Hour).Unix()
	if _, err := p.Action(context.Background(), "complete", json.RawMessage(`{"id":3}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Action(context.Background(), "uncomplete", json.RawMessage(fmt.Sprintf(`{"id":3,"due":%d}`, old))); err != nil {
		t.Fatal(err)
	}
	last := f.posted[len(f.posted)-1]
	if last["done"] != false || last["due_date"] != time.Unix(old, 0).UTC().Format(time.RFC3339) {
		t.Errorf("uncomplete body: %+v", last)
	}
}

func TestAddCreatesTaskInConfiguredProject(t *testing.T) {
	f := newFake()
	p := start(t, f, map[string]any{"projects": []any{"Smarthome"}})
	if !fetch(t, p).CanAdd {
		t.Error("a single configured project allows adding")
	}
	if _, err := p.Action(context.Background(), "add", json.RawMessage(`{"title":"  Milch  "}`)); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 1 || f.created[0]["path"] != "/projects/5/tasks" {
		t.Fatalf("created: %+v", f.created)
	}
	if f.created[0]["body"].(map[string]any)["title"] != "Milch" {
		t.Errorf("title not trimmed: %+v", f.created[0])
	}

	// Without a target project adding is refused and not offered.
	p = start(t, newFake(), map[string]any{})
	if fetch(t, p).CanAdd {
		t.Error("CanAdd must be false without addProject")
	}
	if _, err := p.Action(context.Background(), "add", json.RawMessage(`{"title":"x"}`)); err == nil {
		t.Error("add without a project must fail")
	}
}

func TestActionErrors(t *testing.T) {
	p := start(t, newFake(), map[string]any{})
	for name, c := range map[string]struct{ action, params string }{
		"unknown":      {"explode", `{}`},
		"no id":        {"complete", `{}`},
		"missing task": {"complete", `{"id":12345}`},
		"empty title":  {"add", `{"title":" "}`},
	} {
		if _, err := p.Action(context.Background(), c.action, json.RawMessage(c.params)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestStartValidation(t *testing.T) {
	conn := map[string]any{"url": "https://x.example", "token": "t"}
	for name, env := range map[string]widget.Env{
		"no connection": {Settings: map[string]any{}},
		"bad url":       {Connection: map[string]any{"url": "ftp://x", "token": "t"}},
		"no token":      {Connection: map[string]any{"url": "https://x.example"}},
		"bad due":       {Connection: conn, Settings: map[string]any{"due": "someday"}},
		"bad limit":     {Connection: conn, Settings: map[string]any{"limit": 0}},
	} {
		if err := (&Provider{}).Start(context.Background(), env); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestUnknownProjectIsAnError(t *testing.T) {
	p := start(t, newFake(), map[string]any{"projects": []any{"Nope"}})
	if _, err := p.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "Nope") {
		t.Fatalf("want a clear error naming the project, got %v", err)
	}
}
