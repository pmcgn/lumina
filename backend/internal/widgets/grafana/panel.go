package grafana

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// panelDef is what the widget needs to know about a Grafana panel: its queries
// and how to display the result.
type panelDef struct {
	title       string
	ptype       string
	display     Display // derived from the panel type; "" if the type is not supported
	targets     []map[string]any
	datasource  any // panel level datasource: nil, a name, or {type, uid}
	unit        string
	decimals    *int
	min, max    *float64
	thresholds  []Threshold
	from        string // the dashboard's time range start, e.g. now-6h
	fixedColor  string
	colorByName map[string]string
	fetched     time.Time
}

type dashboardResp struct {
	Dashboard struct {
		Title string `json:"title"`
		Time  struct {
			From string `json:"from"`
		} `json:"time"`
		Panels []panelJSON `json:"panels"`
	} `json:"dashboard"`
}

type panelJSON struct {
	ID          int              `json:"id"`
	Type        string           `json:"type"`
	Title       string           `json:"title"`
	Datasource  any              `json:"datasource"`
	Targets     []map[string]any `json:"targets"`
	Panels      []panelJSON      `json:"panels"` // inside a row
	FieldConfig struct {
		Defaults struct {
			Unit     string   `json:"unit"`
			Decimals *float64 `json:"decimals"`
			Min      *float64 `json:"min"`
			Max      *float64 `json:"max"`
			Color    struct {
				Mode       string `json:"mode"`
				FixedColor string `json:"fixedColor"`
			} `json:"color"`
			Thresholds struct {
				Steps []struct {
					Value *float64 `json:"value"`
					Color string   `json:"color"`
				} `json:"steps"`
			} `json:"thresholds"`
		} `json:"defaults"`
		Overrides []struct {
			Matcher struct {
				ID      string `json:"id"`
				Options any    `json:"options"`
			} `json:"matcher"`
			Properties []struct {
				ID    string `json:"id"`
				Value any    `json:"value"`
			} `json:"properties"`
		} `json:"overrides"`
	} `json:"fieldConfig"`
}

// displayFor maps a Grafana panel type to how the widget draws it.
func displayFor(panelType string) Display {
	switch panelType {
	case "timeseries", "trend", "barchart", "graph", "xychart", "histogram", "candlestick", "state-timeline", "status-history":
		return DisplayChart
	case "stat", "singlestat":
		return DisplayStat
	case "gauge", "bargauge":
		return DisplayGauge
	}
	return ""
}

// loadPanel reads a dashboard and finds the panel.
func (p *Provider) loadPanel(ctx context.Context) (*panelDef, error) {
	var dash dashboardResp
	path := "/api/dashboards/uid/" + url.PathEscape(p.s.Dashboard)
	err := p.do(ctx, http.MethodGet, path, nil, &dash)
	if err != nil {
		// not a uid: look the dashboard up by title
		uid, serr := p.findDashboardUID(ctx, p.s.Dashboard)
		if serr != nil {
			return nil, serr
		}
		if err := p.do(ctx, http.MethodGet, "/api/dashboards/uid/"+url.PathEscape(uid), nil, &dash); err != nil {
			return nil, err
		}
	}

	panel, titles := findPanel(dash.Dashboard.Panels, string(p.s.Panel))
	if panel == nil {
		return nil, fmt.Errorf("panel %q not found in dashboard %q (available: %s)", p.s.Panel, dash.Dashboard.Title, strings.Join(titles, ", "))
	}
	d := &panelDef{
		title:      panel.Title,
		ptype:      panel.Type,
		display:    displayFor(panel.Type),
		targets:    panel.Targets,
		datasource: panel.Datasource,
		unit:       panel.FieldConfig.Defaults.Unit,
		min:        panel.FieldConfig.Defaults.Min,
		max:        panel.FieldConfig.Defaults.Max,
		from:       dash.Dashboard.Time.From,
		fetched:    p.now(),
	}
	if dec := panel.FieldConfig.Defaults.Decimals; dec != nil {
		n := int(*dec)
		d.decimals = &n
	}
	for _, st := range panel.FieldConfig.Defaults.Thresholds.Steps {
		d.thresholds = append(d.thresholds, Threshold{Value: st.Value, Color: normalizeColor(st.Color)})
	}
	if c := panel.FieldConfig.Defaults.Color; c.Mode == "fixed" {
		d.fixedColor = normalizeColor(c.FixedColor)
	}
	// per-series colors from overrides ("byName" matcher with a fixed color)
	for _, o := range panel.FieldConfig.Overrides {
		name, _ := o.Matcher.Options.(string)
		if o.Matcher.ID != "byName" || name == "" {
			continue
		}
		for _, prop := range o.Properties {
			if prop.ID != "color" {
				continue
			}
			if v, ok := prop.Value.(map[string]any); ok && v["mode"] == "fixed" {
				if c, _ := v["fixedColor"].(string); normalizeColor(c) != "" {
					if d.colorByName == nil {
						d.colorByName = map[string]string{}
					}
					d.colorByName[name] = normalizeColor(c)
				}
			}
		}
	}
	return d, nil
}

// findPanel looks a panel up by id or title (case-insensitive), also inside
// rows. titles lists what exists, for the error message.
func findPanel(panels []panelJSON, ref string) (*panelJSON, []string) {
	var titles []string
	var walk func(ps []panelJSON) *panelJSON
	walk = func(ps []panelJSON) *panelJSON {
		for i := range ps {
			p := &ps[i]
			if p.Type != "row" {
				if strconv.Itoa(p.ID) == ref || strings.EqualFold(p.Title, ref) {
					return p
				}
				titles = append(titles, fmt.Sprintf("%q (id %d)", p.Title, p.ID))
			}
			if found := walk(p.Panels); found != nil {
				return found
			}
		}
		return nil
	}
	found := walk(panels)
	sort.Strings(titles)
	return found, titles
}

// findDashboardUID resolves a dashboard title to its uid.
func (p *Provider) findDashboardUID(ctx context.Context, title string) (string, error) {
	var hits []struct {
		UID   string `json:"uid"`
		Title string `json:"title"`
	}
	q := url.Values{"type": {"dash-db"}, "query": {title}}
	if err := p.do(ctx, http.MethodGet, "/api/search?"+q.Encode(), nil, &hits); err != nil {
		return "", err
	}
	var exact []string
	var names []string
	for _, h := range hits {
		names = append(names, fmt.Sprintf("%q", h.Title))
		if strings.EqualFold(h.Title, title) {
			exact = append(exact, h.UID)
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0], nil
	case len(exact) > 1:
		return "", fmt.Errorf("dashboard title %q is ambiguous, use its uid", title)
	}
	if len(names) > 0 {
		return "", fmt.Errorf("dashboard %q not found (similar: %s)", title, strings.Join(names, ", "))
	}
	return "", fmt.Errorf("dashboard %q not found (neither uid nor title)", title)
}

// --- datasources ----------------------------------------------------------

type dsInfo struct {
	UID       string `json:"uid"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	IsDefault bool   `json:"isDefault"`
}

func (p *Provider) datasources(ctx context.Context) ([]dsInfo, error) {
	if p.dsList != nil && p.now().Sub(p.dsFetched) < definitionTTL {
		return p.dsList, nil
	}
	var list []dsInfo
	if err := p.do(ctx, http.MethodGet, "/api/datasources", nil, &list); err != nil {
		return nil, err
	}
	p.dsList, p.dsFetched = list, p.now()
	return list, nil
}

// resolveDatasource turns a panel/target datasource reference (nil, a name or
// uid, or {type, uid}) into a concrete datasource.
func (p *Provider) resolveDatasource(ctx context.Context, ref any) (dsInfo, error) {
	list, err := p.datasources(ctx)
	if err != nil {
		return dsInfo{}, err
	}
	var uid, name string
	switch v := ref.(type) {
	case nil:
	case string:
		uid, name = v, v
	case map[string]any:
		uid, _ = v["uid"].(string)
		if t, _ := v["type"].(string); uid == "" {
			name = t
		}
	default:
		return dsInfo{}, fmt.Errorf("unsupported datasource reference %v", ref)
	}
	if strings.HasPrefix(uid, "$") || strings.HasPrefix(name, "$") {
		return dsInfo{}, fmt.Errorf("the datasource %q is a dashboard variable, which is not supported: set the widget's datasource explicitly", firstNonEmpty(uid, name))
	}
	if uid == "" && name == "" {
		for _, d := range list {
			if d.IsDefault {
				return d, nil
			}
		}
		return dsInfo{}, fmt.Errorf("no default datasource configured in Grafana")
	}
	for _, d := range list {
		if d.UID == uid || strings.EqualFold(d.Name, name) {
			return d, nil
		}
	}
	// a type name (legacy panels) matches the first datasource of that type
	for _, d := range list {
		if d.Type == name {
			return d, nil
		}
	}
	return dsInfo{}, fmt.Errorf("datasource %q not found in Grafana", firstNonEmpty(uid, name))
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
