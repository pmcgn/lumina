package mqtt

// Wire model: this file is converted to TypeScript by tygo (frontend/src/widgets/mqtt/model.gen.ts).
// Keep it free of anything that is not part of the data sent to the browser.
// The data has the same shape as the one of grafana.panel: both widgets are drawn by the same component.

// Display is how the value is drawn.
type Display string

const (
	DisplayChart   Display = "chart"   // time series of the values received since start
	DisplayStat    Display = "stat"    // one big value with a sparkline
	DisplayGauge   Display = "gauge"   // value on a dial between min and max
	DisplayBattery Display = "battery" // value as a battery icon, 0-100 (min/max are ignored)
)

// BatteryStyle is the look of the battery icon.
type BatteryStyle string

const (
	BatteryStyleClassic BatteryStyle = "classic" // solid icon, red / orange / green by charge (or the thresholds)
	BatteryStyleMinimal BatteryStyle = "minimal" // thin outline, one accent color
)

// BatteryValue is where the number of a battery is shown.
type BatteryValue string

const (
	BatteryValueNone   BatteryValue = "none"   // icon only
	BatteryValueInline BatteryValue = "inline" // inside the icon
	BatteryValueSide   BatteryValue = "side"   // next to the icon
)

// BatteryOrientation is how the battery icon is laid out.
type BatteryOrientation string

const (
	BatteryOrientationHorizontal BatteryOrientation = "horizontal"
	BatteryOrientationVertical   BatteryOrientation = "vertical"
)

// Data is the widget's data model.
type Data struct {
	Title      string      `json:"title"`
	Display    Display     `json:"display"`
	Unit       string      `json:"unit,omitempty"`     // unit id, e.g. "watt", "percent", "celsius"
	Decimals   *int        `json:"decimals,omitempty"` // fixed number of decimals; absent = automatic
	Min        *float64    `json:"min,omitempty"`
	Max        *float64    `json:"max,omitempty"`
	Thresholds []Threshold `json:"thresholds,omitempty"` // ascending; color of the highest step at or below the value
	Series     []Series    `json:"series"`
	From       int64       `json:"from"`               // unix ms: start of the shown time range
	To         int64       `json:"to"`                 // unix ms: end of the shown time range
	Warnings   []string    `json:"warnings,omitempty"` // e.g. the last message was not a number
}

// Threshold is a color step. The base step has no value.
type Threshold struct {
	Value *float64 `json:"value"`
	Color string   `json:"color"` // CSS color
}

// Series is the received value over time (a stat/gauge shows its last point).
type Series struct {
	Name   string  `json:"name"` // empty: the value is not labeled
	Points []Point `json:"points"`
}

type Point struct {
	T int64   `json:"t"` // unix ms
	V float64 `json:"v"`
}

// Settings of an mqtt.value widget (the YAML under "settings:"). The broker,
// credentials and TLS options belong to the connection, not to the widget.
type Settings struct {
	Topic     string  `json:"topic"`               // topic to subscribe to; no wildcards
	ValuePath string  `json:"valuePath,omitempty"` // path into a JSON payload, e.g. "value" or "data.soc"; default: the payload itself, or "value" if it is a JSON object
	Title     string  `json:"title,omitempty"`     // heading; default: the topic
	Display   Display `json:"display,omitempty"`   // stat | gauge | chart | battery; default stat
	Range     string  `json:"range,omitempty"`     // time range of a chart or sparkline, e.g. 30m, 6h; default 6h

	// Battery display only.
	BatteryStyle       BatteryStyle       `json:"batteryStyle,omitempty"`       // classic | minimal; default classic
	BatteryValue       BatteryValue       `json:"batteryValue,omitempty"`       // none | inline | side; default side
	BatteryOrientation BatteryOrientation `json:"batteryOrientation,omitempty"` // horizontal | vertical; default horizontal

	Unit       string      `json:"unit,omitempty"`
	Decimals   *int        `json:"decimals,omitempty"`
	Min        *float64    `json:"min,omitempty"`
	Max        *float64    `json:"max,omitempty"`
	Thresholds []Threshold `json:"thresholds,omitempty"` // color steps, e.g. [{color: red}, {value: 20, color: green}]
}
