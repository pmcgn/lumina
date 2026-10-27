package grafana

import "github.com/pmcgn/lumina/internal/widget"

// Wire model: this file is converted to TypeScript by tygo (frontend/src/widgets/grafana/model.gen.ts).
// Keep it free of anything that is not part of the data sent to the browser.

// Display is how a panel is drawn.
type Display string

const (
	DisplayChart Display = "chart" // time series, one line per series
	DisplayStat  Display = "stat"  // one big value with a sparkline
	DisplayGauge Display = "gauge" // value on a dial between min and max
)

// Data is the widget's data model.
type Data struct {
	Title      string      `json:"title"`
	Display    Display     `json:"display"`
	Unit       string      `json:"unit,omitempty"`     // Grafana unit id, e.g. "watt", "percent", "celsius"
	Decimals   *int        `json:"decimals,omitempty"` // fixed number of decimals; absent = automatic
	Min        *float64    `json:"min,omitempty"`
	Max        *float64    `json:"max,omitempty"`
	Thresholds []Threshold `json:"thresholds,omitempty"` // ascending; color of the highest step at or below the value
	Series     []Series    `json:"series"`
	From       int64       `json:"from"`               // unix ms: start of the shown time range
	To         int64       `json:"to"`                 // unix ms: end of the shown time range
	Warnings   []string    `json:"warnings,omitempty"` // queries that failed while others worked
}

// Threshold is a color step. The base step has no value.
type Threshold struct {
	Value *float64 `json:"value"`
	Color string   `json:"color"` // CSS color
}

// Series is one line of a chart (or the source of a stat/gauge value: its last point).
type Series struct {
	Name   string  `json:"name"`
	Color  string  `json:"color,omitempty"` // CSS color from the panel; absent = palette
	Points []Point `json:"points"`
}

type Point struct {
	T int64   `json:"t"` // unix ms
	V float64 `json:"v"`
}

// Settings of a grafana.panel widget (the YAML under "settings:"). Either
// reference an existing dashboard panel (dashboard + panel), or give a
// datasource and queries (as Grafana stores them in a panel's "targets").
type Settings struct {
	// Panel mode: the widget runs the queries of an existing panel and takes its
	// unit, thresholds and range from it.
	Dashboard string            `json:"dashboard,omitempty"` // dashboard uid or title
	Panel     widget.FlexString `json:"panel,omitempty"`     // panel id or title

	// Query mode.
	Datasource string           `json:"datasource,omitempty"` // datasource uid or name
	Queries    []map[string]any `json:"queries,omitempty"`    // Grafana query objects, e.g. {refId: A, expr: "up"}

	Title   string  `json:"title,omitempty"`   // heading; default: the panel title
	Display Display `json:"display,omitempty"` // chart | stat | gauge; default: from the panel type
	Range   string  `json:"range,omitempty"`   // time range, e.g. 30m, 6h, 7d; default: the dashboard's range, else 6h

	// Overrides (and the only source in query mode).
	Unit     string   `json:"unit,omitempty"`
	Decimals *int     `json:"decimals,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Legend   *bool    `json:"legend,omitempty"` // names and values below a chart; default: with several series

	IntervalSeconds *int `json:"intervalSeconds,omitempty"` // refresh interval, default 60, minimum 10
	MaxPoints       *int `json:"maxPoints,omitempty"`       // points per series sent to the browser, 50-1000, default 300
}
