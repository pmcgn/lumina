// Package widgets_test checks, for every widget type at once, that settings
// are validated strictly: a typo or a wrong type is reported with a helpful
// message instead of being silently ignored.
package widgets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmcgn/lumina/internal/config"
	"github.com/pmcgn/lumina/internal/widget"
	_ "github.com/pmcgn/lumina/internal/widgets/calendar"
	_ "github.com/pmcgn/lumina/internal/widgets/clock"
	_ "github.com/pmcgn/lumina/internal/widgets/grafana"
	_ "github.com/pmcgn/lumina/internal/widgets/openhab"
	_ "github.com/pmcgn/lumina/internal/widgets/vikunja"
	_ "github.com/pmcgn/lumina/internal/widgets/weather"
)

type m = map[string]any

var conns = map[string]map[string]any{"family": {"url": "https://example.com/family.ics"}}

func TestEveryWidgetTypeValidatesItsSettings(t *testing.T) {
	cases := []struct {
		typ      string
		settings m
		want     string // "" = valid; otherwise a part of the expected error
	}{
		// valid
		{"core.clock", m{"timezone": "Europe/Berlin", "showSeconds": true, "label": "Berlin"}, ""},
		{"core.clock", nil, ""},
		{"weather.forecast", m{"location": "Köln"}, ""},
		{"weather.forecast", m{"latitude": 50.9, "longitude": 6.9, "label": "Zuhause"}, ""},
		{"vikunja.tasks", m{"projects": []any{"Smarthome", 5}, "due": "today", "limit": 10, "addProject": "Inbox"}, ""},
		{"vikunja.tasks", m{"projects": "Inbox", "labels": "home"}, ""},
		{"openhab.sitemap", m{"sitemap": "Favs", "title": "Favoriten"}, ""},
		{"calendar.agenda", m{"calendars": []any{"family", m{"connection": "family", "name": "Familie", "color": "#fff"}}, "days": 7}, ""},

		{"grafana.panel", m{"dashboard": "PV", "panel": "Verbrauch", "range": "24h", "display": "stat", "unit": "watt", "decimals": 1}, ""},
		{"grafana.panel", m{"dashboard": "PV", "panel": 34}, ""},
		{"grafana.panel", m{"datasource": "InfluxDB", "queries": []any{m{"refId": "A", "query": "x"}}, "title": "T"}, ""},

		// typos name the valid settings
		{"core.clock", m{"timezon": "UTC"}, `unknown setting "timezon"`},
		{"weather.forecast", m{"locaton": "Köln"}, `unknown setting "locaton"`},
		{"vikunja.tasks", m{"dueTo": "today"}, `unknown setting "dueTo"`},
		{"openhab.sitemap", m{"sitemap": "Favs", "sitemaps": "x"}, `unknown setting "sitemaps"`},
		{"calendar.agenda", m{"calendars": []any{"family"}, "day": 7}, `unknown setting "day"`},

		{"grafana.panel", m{"dashboard": "PV", "panel": 1, "rang": "6h"}, `unknown setting "rang"`},

		// wrong types
		{"core.clock", m{"showSeconds": "yes"}, `setting "showSeconds"`},
		{"vikunja.tasks", m{"limit": "ten"}, `setting "limit"`},
		{"calendar.agenda", m{"calendars": []any{"family"}, "days": "many"}, `setting "days"`},
		{"weather.forecast", m{"latitude": "north", "longitude": 6.9}, `setting "latitude"`},

		// invalid values
		{"core.clock", m{"timezone": "Mars/Olympus"}, `setting "timezone"`},
		{"vikunja.tasks", m{"due": "someday"}, `setting "due"`},
		{"vikunja.tasks", m{"limit": 0}, `setting "limit"`},
		{"calendar.agenda", m{"calendars": []any{"family"}, "days": 365}, `setting "days"`},
		{"calendar.agenda", m{"calendars": []any{"family"}, "weekStart": "tuesday"}, `setting "weekStart"`},
		{"weather.forecast", m{"latitude": 123.0, "longitude": 0.0}, "out of range"},

		{"grafana.panel", m{"dashboard": "PV", "panel": 1, "display": "pie"}, `setting "display"`},
		{"grafana.panel", m{"dashboard": "PV", "panel": 1, "range": "yesterday"}, `setting "range"`},
		{"grafana.panel", m{"dashboard": "PV", "panel": 1, "maxPoints": 5}, `setting "maxPoints"`},
		{"grafana.panel", m{"dashboard": "PV", "panel": 1, "min": 10, "max": 5}, `"min" must be smaller`},
		{"grafana.panel", m{"dashboard": "PV", "panel": 1, "decimals": 20}, `setting "decimals"`},

		// required values
		{"openhab.sitemap", m{}, `"sitemap" is required`},
		{"weather.forecast", m{}, "set either latitude and longitude, or location"},
		{"weather.forecast", m{"latitude": 50.0}, "together"},
		{"calendar.agenda", m{}, `setting "calendars"`},

		{"grafana.panel", m{}, "set dashboard and panel"},
		{"grafana.panel", m{"dashboard": "PV"}, `"panel" is required`},
		{"grafana.panel", m{"panel": 3}, `"dashboard" is required`},
		{"grafana.panel", m{"datasource": "x"}, `"queries" is required`},
		{"grafana.panel", m{"queries": []any{m{"q": 1}}}, `"datasource" is required`},
		{"grafana.panel", m{"dashboard": "PV", "panel": 1, "datasource": "x"}, "not both"},

		// references are checked against the configured connections
		{"calendar.agenda", m{"calendars": []any{"nope"}}, `unknown connection "nope"`},
		{"calendar.agenda", m{"calendars": []any{42}}, "connection name"},
		{"calendar.agenda", m{"calendars": []any{m{"connection": "family", "colour": "red"}}}, "connection name"},
	}
	for _, c := range cases {
		name := c.typ + " " + strings.ReplaceAll(c.want, " ", "_")
		t.Run(name, func(t *testing.T) {
			err := widget.ValidateSettings(c.typ, c.settings, conns)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("valid settings rejected: %v", err)
			case c.want != "" && err == nil:
				t.Fatalf("expected an error containing %q", c.want)
			case c.want != "" && !strings.Contains(err.Error(), c.want):
				t.Fatalf("error %q should contain %q", err, c.want)
			}
		})
	}
}

func TestEveryRegisteredWidgetImplementsTheValidator(t *testing.T) {
	for _, typ := range widget.Types() {
		p, _ := widget.New(typ)
		if _, ok := p.(widget.Validator); !ok {
			t.Errorf("%s does not implement widget.Validator: bad settings would only show up at runtime", typ)
		}
	}
}

func TestRepositoryConfigsAreValid(t *testing.T) {
	for _, file := range []string{"config.example.yaml", "config.demo.yaml"} {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "..", "..", "config", file))
			if err != nil {
				t.Fatal(err)
			}
			anyEnv := func(string) (string, bool) { return "x", true }
			if _, err := config.Parse(raw, anyEnv); err != nil {
				t.Fatalf("%s is not valid: %v", file, err)
			}
		})
	}
}
