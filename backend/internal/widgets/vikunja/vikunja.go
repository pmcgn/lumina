// Package vikunja is the task list widget for Vikunja: it lists the open tasks
// (not just a count), marks overdue ones, and can complete tasks and add new
// ones.
package vikunja

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pmcgn/lumina/internal/httpx"
	"github.com/pmcgn/lumina/internal/widget"
)

const Type = "vikunja.tasks"

const (
	pageSize        = 50 // Vikunja's default maximum
	maxPages        = 10
	defaultLimit    = 50
	defaultInterval = 60 * time.Second
	minInterval     = 15 * time.Second
)

func init() {
	widget.Register(Type, func() widget.Provider { return &Provider{client: httpx.NewClient(), now: time.Now} })
}

type Provider struct {
	client *http.Client
	now    func() time.Time

	base   string
	header http.Header
	loc    *time.Location

	projects []string // names or ids from settings
	labels   []string
	window   string // all | overdue | today | week
	filter   string // extra Vikunja filter expression
	limit    int
	interval time.Duration
	addTo    string // project name or id for new tasks
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
		return errors.New("connection.token: missing")
	}
	p.base = raw + "/api/v1"
	p.header = http.Header{"Authorization": {"Bearer " + token}}
	p.loc = env.Location
	if p.loc == nil {
		p.loc = time.Local
	}

	s, err := parseSettings(env.Settings)
	if err != nil {
		return err
	}
	p.projects = []string(s.Projects)
	p.labels = []string(s.Labels)
	p.filter = s.Filter
	p.addTo = s.AddProject
	if p.addTo == "" && len(p.projects) == 1 {
		p.addTo = p.projects[0]
	}
	p.window = "all"
	if s.Due != "" {
		p.window = s.Due
	}
	p.limit = defaultLimit
	if s.Limit != nil {
		p.limit = *s.Limit
	}
	p.interval = defaultInterval
	if s.IntervalSeconds != nil {
		p.interval = time.Duration(*s.IntervalSeconds) * time.Second
	}
	if p.interval < minInterval {
		p.interval = minInterval
	}
	return nil
}

// parseSettings decodes and validates the widget settings.
func parseSettings(m map[string]any) (Settings, error) {
	var s Settings
	if err := widget.DecodeSettings(m, &s); err != nil {
		return s, err
	}
	switch s.Due {
	case "", "all", "overdue", "today", "week":
	default:
		return s, fmt.Errorf("setting %q: %q is invalid (all, overdue, today, week)", "due", s.Due)
	}
	if s.Limit != nil && *s.Limit < 1 {
		return s, fmt.Errorf("setting %q: must be >= 1", "limit")
	}
	return s, nil
}

func (p *Provider) ValidateSettings(m map[string]any, _ map[string]map[string]any) error {
	_, err := parseSettings(m)
	return err
}

func (p *Provider) Interval() time.Duration { return p.interval }

func (p *Provider) Fetch(ctx context.Context) (any, error) {
	projects, err := p.listProjects(ctx)
	if err != nil {
		return nil, err
	}
	wantProjects, err := resolveProjects(p.projects, projects)
	if err != nil {
		return nil, err
	}
	tasks, err := p.listOpenTasks(ctx)
	if err != nil {
		return nil, err
	}

	now := p.now()
	var out []Task
	for _, t := range tasks {
		if len(wantProjects) > 0 && !wantProjects[t.ProjectID] {
			continue
		}
		if !hasLabels(t, p.labels) || !inWindow(t, p.window, now, p.loc) {
			continue
		}
		t.Project = projects[t.ProjectID]
		out = append(out, t)
	}
	sortTasks(out, now)

	d := Data{Tasks: out, Total: len(out)}
	if len(d.Tasks) > p.limit {
		d.Tasks = d.Tasks[:p.limit]
	}
	if d.Tasks == nil {
		d.Tasks = []Task{}
	}
	if _, err := resolveProject(p.addTo, projects); err == nil && p.addTo != "" {
		d.CanAdd = true
	}
	return d, nil
}

// --- Vikunja API ---------------------------------------------------------

type apiTask struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Done        bool      `json:"done"`
	DueDate     time.Time `json:"due_date"`
	ProjectID   int64     `json:"project_id"`
	Priority    int       `json:"priority"`
	RepeatAfter int64     `json:"repeat_after"`
	RepeatMode  int       `json:"repeat_mode"`
	Labels      []struct {
		Title    string `json:"title"`
		HexColor string `json:"hex_color"`
	} `json:"labels"`
}

func (a apiTask) toTask() Task {
	t := Task{ID: a.ID, Title: a.Title, ProjectID: a.ProjectID, Priority: a.Priority, Recurring: a.RepeatAfter > 0 || a.RepeatMode > 0}
	if a.DueDate.Year() > 1 {
		t.Due = a.DueDate.Unix()
	}
	for _, l := range a.Labels {
		t.Labels = append(t.Labels, Label{Title: l.Title, Color: strings.TrimPrefix(l.HexColor, "#")})
	}
	return t
}

func (p *Provider) do(ctx context.Context, method, path string, in, out any) error {
	return httpx.DoJSON(ctx, p.client, method, p.base+path, p.header, in, out)
}

// listOpenTasks returns all open tasks the token can see (paginated).
func (p *Provider) listOpenTasks(ctx context.Context) ([]Task, error) {
	filter := "done = false"
	if p.filter != "" {
		filter = "(" + filter + ") && (" + p.filter + ")"
	}
	var all []Task
	for page := 1; page <= maxPages; page++ {
		q := url.Values{"filter": {filter}, "per_page": {strconv.Itoa(pageSize)}, "page": {strconv.Itoa(page)}}
		var batch []apiTask
		if err := p.do(ctx, http.MethodGet, "/tasks?"+q.Encode(), nil, &batch); err != nil {
			return nil, err
		}
		for _, a := range batch {
			if !a.Done {
				all = append(all, a.toTask())
			}
		}
		if len(batch) < pageSize {
			break
		}
	}
	return all, nil
}

// listProjects returns id -> title of all (non-archived) projects.
func (p *Provider) listProjects(ctx context.Context) (map[int64]string, error) {
	out := map[int64]string{}
	for page := 1; page <= maxPages; page++ {
		q := url.Values{"per_page": {strconv.Itoa(pageSize)}, "page": {strconv.Itoa(page)}}
		var batch []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		}
		if err := p.do(ctx, http.MethodGet, "/projects?"+q.Encode(), nil, &batch); err != nil {
			return nil, err
		}
		for _, pr := range batch {
			out[pr.ID] = pr.Title
		}
		if len(batch) < pageSize {
			break
		}
	}
	return out, nil
}

// --- actions -------------------------------------------------------------

// Action implements widget.Actor: "complete", "uncomplete" and "add".
func (p *Provider) Action(ctx context.Context, name string, params json.RawMessage) (any, error) {
	switch name {
	case "complete":
		var a struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(params, &a); err != nil || a.ID <= 0 {
			return nil, errors.New("complete: id required")
		}
		return p.setDone(ctx, a.ID, true, nil)
	case "uncomplete":
		var a struct {
			ID  int64  `json:"id"`
			Due *int64 `json:"due"` // restore the previous due date (recurring tasks)
		}
		if err := json.Unmarshal(params, &a); err != nil || a.ID <= 0 {
			return nil, errors.New("uncomplete: id required")
		}
		return p.setDone(ctx, a.ID, false, a.Due)
	case "add":
		var a struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(params, &a); err != nil || strings.TrimSpace(a.Title) == "" {
			return nil, errors.New("add: title required")
		}
		return p.add(ctx, strings.TrimSpace(a.Title))
	}
	return nil, fmt.Errorf("unknown action %q", name)
}

type doneResult struct {
	Done bool  `json:"done"`
	Due  int64 `json:"due,omitempty"`
}

// setDone changes the done flag. Vikunja's update replaces the whole task, so
// the task is read first and written back with every field untouched except
// the ones we change; otherwise unrelated fields would be reset.
func (p *Provider) setDone(ctx context.Context, id int64, done bool, due *int64) (doneResult, error) {
	path := "/tasks/" + strconv.FormatInt(id, 10)
	var task map[string]any
	if err := p.getRaw(ctx, path, &task); err != nil {
		return doneResult{}, err
	}
	task["done"] = done
	if due != nil {
		task["due_date"] = time.Unix(*due, 0).UTC().Format(time.RFC3339)
	}
	var updated apiTask
	if err := p.do(ctx, http.MethodPost, path, task, &updated); err != nil {
		return doneResult{}, err
	}
	res := doneResult{Done: updated.Done}
	if updated.DueDate.Year() > 1 {
		res.Due = updated.DueDate.Unix()
	}
	return res, nil
}

// getRaw decodes into a generic map keeping numbers exact.
func (p *Provider) getRaw(ctx context.Context, path string, out *map[string]any) error {
	var raw json.RawMessage
	if err := p.do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	return dec.Decode(out)
}

func (p *Provider) add(ctx context.Context, title string) (any, error) {
	projects, err := p.listProjects(ctx)
	if err != nil {
		return nil, err
	}
	id, err := resolveProject(p.addTo, projects)
	if err != nil || p.addTo == "" {
		return nil, errors.New("no project configured for new tasks (settings.addProject)")
	}
	var created apiTask
	err = p.do(ctx, http.MethodPut, "/projects/"+strconv.FormatInt(id, 10)+"/tasks", map[string]any{"title": title}, &created)
	if err != nil {
		return nil, err
	}
	return map[string]int64{"id": created.ID}, nil
}
