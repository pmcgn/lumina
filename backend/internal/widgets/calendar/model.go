package calendar

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Wire model: this file is converted to TypeScript by tygo (frontend/src/widgets/calendar/model.gen.ts).
// Keep it free of anything that is not part of the data sent to the browser.

// Data is the widget's data model (mirrored in frontend/src/widgets/calendar).
type Data struct {
	Timezone  string           `json:"timezone"`
	WeekStart int              `json:"weekStart"` // 0 = Sunday, 1 = Monday
	Calendars []Info           `json:"calendars"`
	Events    []Event          `json:"events"` // agenda: from the start of today
	Marks     map[string][]int `json:"marks"`  // "YYYY-MM-DD" -> calendar indexes with events (month grid)
	MonthFrom string           `json:"monthFrom"`
}

type Info struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	Error string `json:"error,omitempty"` // set if the feed could not be refreshed (cached data may be shown)
}

type Event struct {
	Title    string `json:"title"`
	Start    int64  `json:"start"` // unix seconds; all-day: local midnight
	End      int64  `json:"end"`   // exclusive
	AllDay   bool   `json:"allDay,omitempty"`
	Location string `json:"location,omitempty"`
	Cal      int    `json:"cal"`
}

// Settings of a calendar.agenda widget (the YAML under "settings:").
type Settings struct {
	Title     string        `json:"title,omitempty"`     // heading; default "Termine"/"Calendar"
	Calendars []CalendarRef `json:"calendars"`           // the feeds to merge (required)
	Days      *int          `json:"days,omitempty"`      // how far the agenda looks ahead, 1-60, default 14
	Limit     *int          `json:"limit,omitempty"`     // maximum agenda entries, default 40
	ShowPast  bool          `json:"showPast,omitempty"`  // keep today's finished events
	ShowMonth *bool         `json:"showMonth,omitempty"` // month overview; default: automatic by card size

	IntervalMinutes *int   `json:"intervalMinutes,omitempty"` // refresh interval, default 10
	WeekStart       string `json:"weekStart,omitempty"`       // monday (default) | sunday
}

// CalendarRef names one calendar connection. In the YAML it may be written as
// just the connection name or as an object.
type CalendarRef struct {
	Connection string `json:"connection"`
	Name       string `json:"name,omitempty"`  // shown in the legend; default: the connection name
	Color      string `json:"color,omitempty"` // CSS color; default: from a palette
}

func (c *CalendarRef) UnmarshalJSON(b []byte) error {
	var name string
	if json.Unmarshal(b, &name) == nil {
		*c = CalendarRef{Connection: name}
		return nil
	}
	type plain CalendarRef // avoids recursing into this method
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var p plain
	if err := dec.Decode(&p); err != nil {
		return fmt.Errorf("a calendar must be a connection name or {connection, name, color} (%v)", err)
	}
	*c = CalendarRef(p)
	return nil
}
