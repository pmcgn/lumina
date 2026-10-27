package vikunja

import "github.com/pmcgn/lumina/internal/widget"

// Wire model: this file is converted to TypeScript by tygo (frontend/src/widgets/vikunja/model.gen.ts).
// Keep it free of anything that is not part of the data sent to the browser.

// Data is the widget's data model (mirrored in frontend/src/widgets/vikunja).
type Data struct {
	Tasks  []Task `json:"tasks"`
	Total  int    `json:"total"` // matching tasks before the limit was applied
	CanAdd bool   `json:"canAdd"`
}

type Task struct {
	ID        int64   `json:"id"`
	Title     string  `json:"title"`
	Project   string  `json:"project"`
	ProjectID int64   `json:"projectId"`
	Due       int64   `json:"due,omitempty"` // unix seconds, absent = no due date
	Priority  int     `json:"priority"`
	Recurring bool    `json:"recurring"`
	Labels    []Label `json:"labels,omitempty"`
}

type Label struct {
	Title string `json:"title"`
	Color string `json:"color,omitempty"` // hex without '#'
}

// Settings of a vikunja.tasks widget (the YAML under "settings:").
type Settings struct {
	Title       string `json:"title,omitempty"`       // heading; default "Aufgaben"/"Tasks"
	ShowProject *bool  `json:"showProject,omitempty"` // project name per row; default: when several projects are shown

	Projects   widget.StringList `json:"projects,omitempty"`   // project names or ids; empty = all
	Labels     widget.StringList `json:"labels,omitempty"`     // a task must carry all of these
	Filter     string            `json:"filter,omitempty"`     // extra Vikunja filter expression
	AddProject string            `json:"addProject,omitempty"` // where "add task" creates tasks; default: the only configured project
	Due        string            `json:"due,omitempty"`        // all | overdue | today | week (today and week include overdue)

	Limit           *int `json:"limit,omitempty"`           // maximum rows, default 50
	IntervalSeconds *int `json:"intervalSeconds,omitempty"` // poll interval, default 60, minimum 15
}
