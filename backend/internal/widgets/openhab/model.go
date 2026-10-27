package openhab

// Wire model: this file is converted to TypeScript by tygo (frontend/src/widgets/openhab/model.gen.ts).
// Keep it free of anything that is not part of the data sent to the browser.

// RowKind says how a row is rendered and what can be done with it.
type RowKind string

const (
	RowKindFrame  RowKind = "frame"  // section heading
	RowKindSwitch RowKind = "switch" // on/off tile
	RowKindChoice RowKind = "choice" // buttons from sitemap mappings
	RowKindText   RowKind = "text"   // read-only value
	RowKindSlider RowKind = "slider" // dimmer, blinds, numeric setpoint
)

// Data is the widget's data model (mirrored in frontend/src/widgets/openhab).
type Data struct {
	Title string `json:"title"`
	Rows  []Row  `json:"rows"`
}

type Row struct {
	ID       string    `json:"id"` // sitemap widget id, used by actions
	Kind     RowKind   `json:"kind"`
	Label    string    `json:"label"`
	Value    string    `json:"value,omitempty"`
	Icon     string    `json:"icon,omitempty"`
	On       bool      `json:"on,omitempty"`
	Locked   bool      `json:"locked,omitempty"` // switch that must not be operated (readOnly item)
	Mappings []Mapping `json:"mappings,omitempty"`
	Active   string    `json:"active,omitempty"` // command of the currently active mapping

	// Sliders
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	Step    float64  `json:"step,omitempty"`
	Level   *float64 `json:"level,omitempty"`   // current position; absent if unknown
	Shutter bool     `json:"shutter,omitempty"` // rollershutter: UP/STOP/DOWN buttons are available
}

type Mapping struct {
	Command string `json:"command"`
	Label   string `json:"label"`
}

// Settings of an openhab.sitemap widget (the YAML under "settings:").
type Settings struct {
	Sitemap string `json:"sitemap"`         // name of the openHAB sitemap (required)
	Page    string `json:"page,omitempty"`  // sub-page; default: the sitemap's home page
	Title   string `json:"title,omitempty"` // heading; default: the page title from openHAB
}
