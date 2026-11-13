package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmcgn/lumina/internal/widget"
)

type stub struct{}

func (stub) Start(context.Context, widget.Env) error { return nil }

func init() { widget.Register("test.stub", func() widget.Provider { return stub{} }) }

func lookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestParseValid(t *testing.T) {
	raw := `
connections:
  main: { token: "${TOKEN}" }
pages:
  - id: home
    widgets:
      - { id: a, type: test.stub, connection: main, pos: { x: 0, y: 0, w: 2, h: 1 } }
`
	cfg, err := Parse([]byte(raw), lookup(map[string]string{"TOKEN": "secret"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Connections["main"]["token"]; got != "secret" {
		t.Errorf("env not expanded: %v", got)
	}
	if cfg.Grid.Columns != 6 || cfg.Locale != "de" || cfg.Theme.Mode != "auto" {
		t.Errorf("defaults not applied: %+v", cfg)
	}
}

func TestEnvInCommentsIsIgnored(t *testing.T) {
	raw := "# token: ${NOT_SET}\n  # also: ${NOT_SET}\npages: [{id: p, widgets: [{id: a, type: test.stub}]}]\n"
	if _, err := Parse([]byte(raw), lookup(nil)); err != nil {
		t.Fatalf("variables in full-line comments must not be expanded: %v", err)
	}
	// ...but real values still are.
	if _, err := Parse([]byte("pages: [{id: p, widgets: [{id: a, type: test.stub, settings: {x: '${NOT_SET}'}}]}]"), lookup(nil)); err == nil {
		t.Fatal("undefined variable in a value must still fail")
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]struct{ raw, want string }{
		"missing env":     {"pages: [{id: p, widgets: [{id: a, type: test.stub, settings: {x: '${NOPE}'}}]}]", "NOPE"},
		"unknown type":    {"pages: [{id: p, widgets: [{id: a, type: nope}]}]", "unknown type"},
		"duplicate id":    {"pages: [{id: p, widgets: [{id: a, type: test.stub}, {id: a, type: test.stub, pos: {x: 1}}]}]", "duplicate widget id"},
		"out of grid":     {"pages: [{id: p, widgets: [{id: a, type: test.stub, pos: {x: 5, w: 3}}]}]", "does not fit"},
		"unknown conn":    {"pages: [{id: p, widgets: [{id: a, type: test.stub, connection: x}]}]", "unknown connection"},
		"unknown field":   {"bogus: 1\npages: [{id: p}]", "bogus"},
		"no pages":        {"locale: de", "at least one page"},
		"unsupported loc": {"locale: fr\npages: [{id: p}]", "locale"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.raw), lookup(nil))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

const minimal = "pages: [{id: p, widgets: [{id: a, type: test.stub}]}]\n"

func TestDisplayDefaults(t *testing.T) {
	cfg, err := Parse([]byte(minimal), lookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	d := cfg.Display.Client()
	if d.Rotate.Enabled || d.Rotate.IntervalSeconds != 30 || d.Rotate.PauseSeconds != 60 {
		t.Errorf("rotate defaults: %+v", d.Rotate)
	}
	if d.Night.Enabled || d.Night.Mode != "dim" || d.Night.Dim != 0.75 || d.Night.WakeSeconds != 30 {
		t.Errorf("night defaults: %+v", d.Night)
	}
	if d.SoftReload != "03:30" || d.Lite {
		t.Errorf("soft reload must default to 03:30, lite off: %+v", d)
	}
	if d.Transition.Type != TransitionTypeFade || d.Transition.DurationMs != 300 {
		t.Errorf("transition defaults: %+v", d.Transition)
	}
}

func TestTransitionConfigured(t *testing.T) {
	for _, typ := range []string{"fade", "fly", "slide", "crossfade", "scale", "blur"} {
		cfg, err := Parse([]byte("display: { transition: { type: "+typ+", durationMs: 450 } }\n"+minimal), lookup(nil))
		if err != nil {
			t.Fatal(typ, err)
		}
		if tr := cfg.Display.Client().Transition; string(tr.Type) != typ || tr.DurationMs != 450 {
			t.Errorf("%s: %+v", typ, tr)
		}
	}
	// only the speed set: the type stays fade
	cfg, err := Parse([]byte("display: { transition: { durationMs: 600 } }\n"+minimal), lookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	if tr := cfg.Display.Client().Transition; tr.Type != TransitionTypeFade || tr.DurationMs != 600 {
		t.Errorf("speed only: %+v", tr)
	}
}

func TestDisplayConfigured(t *testing.T) {
	raw := `
display:
  rotate: { enabled: true, intervalSeconds: 12 }
  night: { from: "22:30", to: "06:00", mode: clock, wakeSeconds: 45 }
  softReload: ""
  lite: true
` + minimal
	cfg, err := Parse([]byte(raw), lookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	d := cfg.Display.Client()
	if !d.Rotate.Enabled || d.Rotate.IntervalSeconds != 12 || d.Rotate.PauseSeconds != 60 {
		t.Errorf("rotate: %+v", d.Rotate)
	}
	if !d.Night.Enabled || d.Night.From != "22:30" || d.Night.Mode != "clock" || d.Night.WakeSeconds != 45 {
		t.Errorf("night: %+v", d.Night)
	}
	if d.SoftReload != "" || !d.Lite {
		t.Errorf("an explicit empty softReload disables it: %+v", d)
	}
}

func TestDisplayValidation(t *testing.T) {
	for name, c := range map[string]struct{ yaml, want string }{
		"rotate too fast": {"display: { rotate: { intervalSeconds: 2 } }", "intervalSeconds"},
		"only from":       {"display: { night: { from: '22:00' } }", "both from and to"},
		"bad time":        {"display: { night: { from: '25:00', to: '06:00' } }", "not a time"},
		"bad time format": {"display: { night: { from: '9:00', to: '06:00' } }", "not a time"},
		"same times":      {"display: { night: { from: '22:00', to: '22:00' } }", "must differ"},
		"bad mode":        {"display: { night: { mode: blackout } }", "mode"},
		"too dark":        {"display: { night: { dim: 1 } }", "dim"},
		"bad soft reload": {"display: { softReload: '3am' }", "softReload"},
		"wake too short":  {"display: { night: { wakeSeconds: 1 } }", "wakeSeconds"},
		"bad transition":  {"display: { transition: { type: spin } }", "transition.type"},
		"too fast":        {"display: { transition: { durationMs: 10 } }", "durationMs"},
		"too slow":        {"display: { transition: { durationMs: 5000 } }", "durationMs"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.yaml+"\n"+minimal), lookup(nil))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

type strict struct{}

func (strict) Start(context.Context, widget.Env) error { return nil }
func (strict) ValidateSettings(s map[string]any, _ map[string]map[string]any) error {
	if _, bad := s["typo"]; bad {
		return fmt.Errorf("unknown setting %q", "typo")
	}
	return nil
}

func init() { widget.Register("test.strict", func() widget.Provider { return strict{} }) }

func TestSettingsAreValidatedWhenTheConfigIsLoaded(t *testing.T) {
	good := "pages: [{id: p, widgets: [{id: a, type: test.strict, settings: {fine: 1}}]}]\n"
	if _, err := Parse([]byte(good), lookup(nil)); err != nil {
		t.Fatal(err)
	}
	bad := "pages: [{id: p, widgets: [{id: a, type: test.strict, settings: {typo: 1}}]}]\n"
	_, err := Parse([]byte(bad), lookup(nil))
	if err == nil || !strings.Contains(err.Error(), `widget "a"`) || !strings.Contains(err.Error(), `unknown setting "typo"`) {
		t.Fatalf("a bad setting must be reported at load time, naming the widget: %v", err)
	}
}

func TestWebhookAuthValidation(t *testing.T) {
	page := "\npages:\n  - id: home\n    widgets: []\n"
	cases := []struct{ name, conns, want string }{
		{"ok", `{ v: { webhookSecret: s, webhookUser: u, webhookPassword: p } }`, ""},
		{"secret only", `{ v: { webhookSecret: s } }`, ""},
		{"user without password", `{ v: { webhookUser: u } }`, "set both webhookUser and webhookPassword"},
		{"empty secret", `{ v: { webhookSecret: "" } }`, "connections.v.webhookSecret: must be a non-empty string"},
		{"not a string", `{ v: { webhookSecret: 5 } }`, "connections.v.webhookSecret: must be a non-empty string"},
	}
	for _, c := range cases {
		_, err := Parse([]byte("connections: "+c.conns+page), lookup(nil))
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want %q", c.name, err, c.want)
		}
	}
}

func TestThemeWidgetOpacity(t *testing.T) {
	pages := "pages: [{id: p, widgets: [{id: a, type: test.stub}]}]\n"
	cfg, err := Parse([]byte(pages), lookup(nil))
	if err != nil || cfg.Client().Theme.WidgetOpacity != 1 {
		t.Fatalf("default must be 1: %v %v", err, cfg.Client().Theme.WidgetOpacity)
	}
	cfg, err = Parse([]byte("theme: { widgetOpacity: 0.6 }\n"+pages), lookup(nil))
	if err != nil || cfg.Client().Theme.WidgetOpacity != 0.6 {
		t.Fatalf("0.6: %v %v", err, cfg.Client().Theme.WidgetOpacity)
	}
	for _, v := range []string{"0", "0.05", "1.5", "-1"} {
		if _, err := Parse([]byte("theme: { widgetOpacity: "+v+" }\n"+pages), lookup(nil)); err == nil || !strings.Contains(err.Error(), "widgetOpacity") {
			t.Errorf("%s: want widgetOpacity error, got %v", v, err)
		}
	}
}

func TestThemeStyle(t *testing.T) {
	pages := "pages: [{id: p, widgets: [{id: a, type: test.stub}]}]\n"
	cfg, err := Parse([]byte(pages), lookup(nil))
	if err != nil || cfg.Client().Theme.Style != ThemeStyleFlat {
		t.Fatalf("default must be flat: %v %v", err, cfg.Client().Theme.Style)
	}
	cfg, err = Parse([]byte("theme: { style: glass }\n"+pages), lookup(nil))
	if err != nil || cfg.Client().Theme.Style != ThemeStyleGlass {
		t.Fatalf("glass: %v %v", err, cfg.Client().Theme.Style)
	}
	cfg, err = Parse([]byte("theme: { style: holo }\n"+pages), lookup(nil))
	if err != nil || cfg.Client().Theme.Style != ThemeStyleHolo {
		t.Fatalf("holo: %v %v", err, cfg.Client().Theme.Style)
	}
	if _, err := Parse([]byte("theme: { style: shiny }\n"+pages), lookup(nil)); err == nil || !strings.Contains(err.Error(), "theme.style") {
		t.Errorf("want theme.style error, got %v", err)
	}
}

func TestThemeBackground(t *testing.T) {
	file := filepath.Join(t.TempDir(), "wall.png")
	os.WriteFile(file, []byte("x"), 0o600)
	pages := "pages: [{id: p, widgets: [{id: a, type: test.stub}]}]\n"

	cfg, err := Parse([]byte("theme: { background: { light: dawn, dark: '"+file+"' } }\n"+pages), lookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	bg := cfg.Client().Theme.Background
	if !strings.HasPrefix(bg.Light, "/backgrounds/light?v=") || !strings.HasPrefix(bg.Dark, "/backgrounds/dark?v=") {
		t.Errorf("client URLs: %+v", bg)
	}
	if strings.Contains(bg.Dark, "wall.png") {
		t.Error("the file path must not reach the browser")
	}

	cfg, err = Parse([]byte(pages), lookup(nil))
	if err != nil || cfg.Client().Theme.Background != (BackgroundClient{}) {
		t.Errorf("no background must stay empty: %v %+v", err, cfg.Client().Theme.Background)
	}

	for _, raw := range []string{"{ light: nope }", "{ dark: /does/not/exist.png }"} {
		if _, err := Parse([]byte("theme: { background: "+raw+" }\n"+pages), lookup(nil)); err == nil || !strings.Contains(err.Error(), "theme.background") {
			t.Errorf("%s: want theme.background error, got %v", raw, err)
		}
	}
}
