package openhab

import (
	"strconv"
	"strings"
)

// control is what the backend remembers about a row so actions can be
// validated: only items that are on the rendered page can be commanded.
type control struct {
	item     string
	kind     RowKind
	on       bool
	locked   bool
	mappings []Mapping

	itemType       string
	min, max, step float64
}

// --- openHAB REST model (subset) ----------------------------------------

type pageResp struct {
	ID      string       `json:"id"`
	Title   string       `json:"title"`
	Widgets []widgetResp `json:"widgets"`
}

type widgetResp struct {
	WidgetID   string       `json:"widgetId"`
	Type       string       `json:"type"`
	Label      string       `json:"label"`
	Icon       string       `json:"icon"`
	State      string       `json:"state"`
	Visibility *bool        `json:"visibility"`
	Mappings   []mappingRaw `json:"mappings"`
	MinValue   *float64     `json:"minValue"`
	MaxValue   *float64     `json:"maxValue"`
	StepValue  *float64     `json:"step"`
	Item       *itemResp    `json:"item"`
	Widgets    []widgetResp `json:"widgets"`
}

type mappingRaw struct {
	Command string `json:"command"`
	Label   string `json:"label"`
}

type itemResp struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	State            string `json:"state"`
	Category         string `json:"category"`
	StateDescription *struct {
		ReadOnly bool `json:"readOnly"`
	} `json:"stateDescription"`
}

// buildRows flattens a sitemap page into rows plus the control table.
func buildRows(page pageResp) ([]Row, map[string]control, []string) {
	var rows []Row
	controls := map[string]control{}
	seen := map[string]bool{}
	var items []string

	var walk func(ws []widgetResp)
	walk = func(ws []widgetResp) {
		for _, w := range ws {
			if w.Visibility != nil && !*w.Visibility {
				continue
			}
			if w.Item != nil && w.Item.Name != "" && !seen[w.Item.Name] {
				seen[w.Item.Name] = true
				items = append(items, w.Item.Name)
			}
			label, value := splitLabel(w.Label)
			icon := w.Icon
			if icon == "" && w.Item != nil {
				icon = w.Item.Category
			}
			switch w.Type {
			case "Frame":
				if label != "" {
					rows = append(rows, Row{ID: w.WidgetID, Kind: RowKindFrame, Label: label})
				}
				walk(w.Widgets)
			case "Switch":
				rows = append(rows, switchRow(w, label, value, icon, controls))
			case "Slider":
				rows = append(rows, sliderRow(w, label, value, icon, controls))
			case "Text", "Selection", "Setpoint", "Colorpicker", "Input", "Default", "Group":
				// Values are shown, controls for these are not supported yet.
				rows = append(rows, Row{ID: w.WidgetID, Kind: RowKindText, Label: label, Value: value, Icon: icon})
				walk(w.Widgets)
			}
		}
	}
	walk(page.Widgets)
	return rows, controls, items
}

func switchRow(w widgetResp, label, value, icon string, controls map[string]control) Row {
	row := Row{ID: w.WidgetID, Label: label, Icon: icon}
	item := w.Item
	if item == nil {
		row.Kind, row.Value = RowKindText, value
		return row
	}
	locked := item.StateDescription != nil && item.StateDescription.ReadOnly

	if len(w.Mappings) > 0 {
		row.Kind = RowKindChoice
		for _, m := range w.Mappings {
			row.Mappings = append(row.Mappings, Mapping{Command: m.Command, Label: m.Label})
			if m.Command == item.State {
				row.Active = m.Command
			}
		}
		row.Locked = locked
		controls[w.WidgetID] = control{item: item.Name, kind: RowKindChoice, locked: locked, mappings: row.Mappings}
		return row
	}

	switch item.Type {
	case "Switch", "Dimmer", "Color":
		row.Kind = RowKindSwitch
		row.On = isOn(w, item)
		row.Locked = locked
		controls[w.WidgetID] = control{item: item.Name, kind: RowKindSwitch, on: row.On, locked: locked}
	default:
		// A Switch widget on something that cannot take ON/OFF: show it, do not control it.
		row.Kind, row.Value = RowKindText, value
	}
	return row
}

// sliderRow builds a slider for items that take a numeric level; anything else
// is shown read-only.
func sliderRow(w widgetResp, label, value, icon string, controls map[string]control) Row {
	row := Row{ID: w.WidgetID, Label: label, Icon: icon}
	item := w.Item
	if item == nil || (item.Type != "Dimmer" && item.Type != "Rollershutter" && item.Type != "Color" && item.Type != "Number") {
		row.Kind, row.Value = RowKindText, value
		return row
	}
	// openHAB's defaults when the sitemap gives none.
	min, max, step := 0.0, 100.0, 1.0
	if w.MinValue != nil {
		min = *w.MinValue
	}
	if w.MaxValue != nil {
		max = *w.MaxValue
	}
	if w.StepValue != nil && *w.StepValue > 0 {
		step = *w.StepValue
	}
	if max <= min {
		row.Kind, row.Value = RowKindText, value
		return row
	}
	locked := item.StateDescription != nil && item.StateDescription.ReadOnly
	row.Kind, row.Value = RowKindSlider, value
	row.Min, row.Max, row.Step = min, max, step
	row.Shutter = item.Type == "Rollershutter"
	row.Level = levelOf(w, item)
	row.Locked = locked
	controls[w.WidgetID] = control{item: item.Name, kind: RowKindSlider, locked: locked, itemType: item.Type, min: min, max: max, step: step}
	return row
}

// levelOf reads the numeric position. For color items openHAB reports the
// brightness as the widget state, while the item state is "h,s,b".
func levelOf(w widgetResp, item *itemResp) *float64 {
	for _, s := range []string{w.State, item.State} {
		switch s {
		case "", "NULL", "UNDEF":
			continue
		case "ON":
			v := 100.0
			return &v
		case "OFF":
			v := 0.0
			return &v
		}
		if parts := strings.Split(s, ","); len(parts) == 3 { // HSB
			if v, err := strconv.ParseFloat(parts[2], 64); err == nil {
				return &v
			}
			continue
		}
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return &v
		}
	}
	return nil
}

// isOn derives the on/off state. openHAB computes widget.state for Color
// items; otherwise fall back to the item state (ON, a dimmer level > 0, or
// HSB with brightness > 0).
func isOn(w widgetResp, item *itemResp) bool {
	state := w.State
	if state == "" {
		state = item.State
	}
	switch state {
	case "ON":
		return true
	case "OFF", "NULL", "UNDEF", "":
		return false
	}
	if parts := strings.Split(state, ","); len(parts) == 3 { // HSB
		b, err := strconv.ParseFloat(parts[2], 64)
		return err == nil && b > 0
	}
	n, err := strconv.ParseFloat(state, 64)
	return err == nil && n > 0
}

// splitLabel separates "Licht SZ [OFF]" into the text and the formatted value.
func splitLabel(l string) (label, value string) {
	l = strings.TrimSpace(l)
	if strings.HasSuffix(l, "]") {
		if i := strings.LastIndex(l, " ["); i >= 0 {
			return strings.TrimSpace(l[:i]), l[i+2 : len(l)-1]
		}
		if strings.HasPrefix(l, "[") { // value without a label text
			return "", l[1 : len(l)-1]
		}
	}
	return l, ""
}
