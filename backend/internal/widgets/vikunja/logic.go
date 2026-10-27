package vikunja

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// resolveProject maps a name or numeric id to a project id.
func resolveProject(ref string, projects map[int64]string) (int64, error) {
	if ref == "" {
		return 0, fmt.Errorf("empty project reference")
	}
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		if _, ok := projects[id]; ok {
			return id, nil
		}
	}
	for id, title := range projects {
		if strings.EqualFold(title, ref) {
			return id, nil
		}
	}
	return 0, fmt.Errorf("project %q not found", ref)
}

// resolveProjects returns the set of wanted project ids (nil = all projects).
func resolveProjects(refs []string, projects map[int64]string) (map[int64]bool, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	set := map[int64]bool{}
	for _, r := range refs {
		id, err := resolveProject(r, projects)
		if err != nil {
			return nil, fmt.Errorf("settings.projects: %w", err)
		}
		set[id] = true
	}
	return set, nil
}

// hasLabels reports whether the task carries all wanted labels (case-insensitive).
func hasLabels(t Task, want []string) bool {
	for _, w := range want {
		found := false
		for _, l := range t.Labels {
			if strings.EqualFold(l.Title, w) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// inWindow applies the "due" setting. today/week include overdue tasks;
// tasks without a due date only match "all".
func inWindow(t Task, window string, now time.Time, loc *time.Location) bool {
	if window == "all" {
		return true
	}
	if t.Due == 0 {
		return false
	}
	due := time.Unix(t.Due, 0)
	switch window {
	case "overdue":
		return due.Before(now)
	case "today":
		return due.Before(startOfDay(now, loc).AddDate(0, 0, 1))
	case "week":
		return due.Before(startOfDay(now, loc).AddDate(0, 0, 7))
	}
	return true
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// sortTasks orders: overdue (oldest first), upcoming by due date, then tasks
// without due date; ties by priority (high first), then id.
func sortTasks(ts []Task, now time.Time) {
	rank := func(t Task) int {
		switch {
		case t.Due == 0:
			return 2
		case time.Unix(t.Due, 0).Before(now):
			return 0
		}
		return 1
	}
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		if a.Due != b.Due {
			return a.Due < b.Due
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		return a.ID < b.ID
	})
}
