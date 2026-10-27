// Package config loads and validates the YAML configuration.
package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"gopkg.in/yaml.v3"

	"github.com/pmcgn/lumina/internal/widget"
)

type Config struct {
	Server Server `yaml:"server"`
	Locale Locale `yaml:"locale"`
	// Timezone (IANA) is used by widgets that need "today" etc. Defaults to the process timezone.
	Timezone    string                    `yaml:"timezone"`
	Theme       Theme                     `yaml:"theme"`
	Display     Display                   `yaml:"display"`
	Grid        Grid                      `yaml:"grid"`
	Connections map[string]map[string]any `yaml:"connections"`
	Pages       []Page                    `yaml:"pages"`
}

type Server struct {
	// AllowedOrigins are additional origins (host patterns) allowed to open the
	// WebSocket. The same origin is always allowed. Useful for the Vite dev server.
	AllowedOrigins []string `yaml:"allowedOrigins"`
}

// Display configures how the dashboard behaves on a permanently running screen.
type Display struct {
	Rotate Rotate `yaml:"rotate"`
	Night  Night  `yaml:"night"`
	// SoftReload reloads the page every day at this time (HH:MM) as a safety
	// net for 24/7 operation. Default 03:30; set to "" to disable.
	SoftReload *string `yaml:"softReload"`
	// Lite switches off animations and shadows for weak devices.
	Lite bool `yaml:"lite"`
}

// Night dims the screen (or shows only a clock) between From and To (HH:MM,
// the window may span midnight). It is software only: the tablet's real
// screen and brightness are not controlled.
type Night struct {
	From        string    `yaml:"from"`
	To          string    `yaml:"to"`
	Mode        NightMode `yaml:"mode"`        // dim (default) | clock
	Dim         float64   `yaml:"dim"`         // darkness of the overlay in dim mode, 0..0.95, default 0.75
	WakeSeconds int       `yaml:"wakeSeconds"` // clock mode: a touch shows the dashboard for this long, default 30
}

const defaultSoftReload = "03:30"

// Client returns the browser-facing display settings.
func (d Display) Client() DisplayClient {
	soft := defaultSoftReload
	if d.SoftReload != nil {
		soft = *d.SoftReload
	}
	return DisplayClient{
		Rotate:     d.Rotate,
		Night:      NightClient{Enabled: d.Night.From != "", From: d.Night.From, To: d.Night.To, Mode: d.Night.Mode, Dim: d.Night.Dim, WakeSeconds: d.Night.WakeSeconds},
		SoftReload: soft,
		Lite:       d.Lite,
	}
}

var hhmm = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func (c *Config) validateDisplay(add func(string, ...any)) {
	d := c.Display
	if d.Rotate.IntervalSeconds < 5 {
		add("display.rotate.intervalSeconds: must be >= 5")
	}
	if d.Rotate.PauseSeconds < 0 {
		add("display.rotate.pauseSeconds: must be >= 0")
	}
	n := d.Night
	if (n.From == "") != (n.To == "") {
		add("display.night: set both from and to (or neither)")
	}
	for name, v := range map[string]string{"from": n.From, "to": n.To} {
		if v != "" && !hhmm.MatchString(v) {
			add("display.night.%s: %q is not a time like 23:00", name, v)
		}
	}
	if n.From != "" && n.From == n.To {
		add("display.night: from and to must differ")
	}
	if n.Mode != "dim" && n.Mode != "clock" {
		add("display.night.mode: %q invalid (dim, clock)", n.Mode)
	}
	if n.Dim < 0 || n.Dim > 0.95 {
		add("display.night.dim: %v out of range 0..0.95", n.Dim)
	}
	if n.WakeSeconds < 5 {
		add("display.night.wakeSeconds: must be >= 5")
	}
	if d.SoftReload != nil && *d.SoftReload != "" && !hhmm.MatchString(*d.SoftReload) {
		add("display.softReload: %q is not a time like 03:30 (or \"\" to disable)", *d.SoftReload)
	}
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Load reads, expands ${ENV} references, parses and validates the file.
func Load(path string) (*Config, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := Parse(raw, os.LookupEnv)
	return cfg, raw, err
}

// Parse parses and validates config content. lookup resolves ${VAR} references.
func Parse(raw []byte, lookup func(string) (string, bool)) (*Config, error) {
	var missing []string
	expand := func(m []byte) []byte {
		name := string(envRef.FindSubmatch(m)[1])
		if v, ok := lookup(name); ok {
			return []byte(v)
		}
		missing = append(missing, name)
		return nil
	}
	// Full-line comments are left alone, so commented-out examples can mention ${VARS}.
	lines := bytes.Split(raw, []byte{'\n'})
	for i, l := range lines {
		if !bytes.HasPrefix(bytes.TrimSpace(l), []byte("#")) {
			lines[i] = envRef.ReplaceAllFunc(l, expand)
		}
	}
	expanded := bytes.Join(lines, []byte{'\n'})
	if len(missing) > 0 {
		return nil, fmt.Errorf("undefined environment variable(s): %s", strings.Join(missing, ", "))
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(expanded))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Locale == "" {
		c.Locale = "de"
	}
	if c.Theme.Mode == "" {
		c.Theme.Mode = "auto"
	}
	if c.Display.Rotate.IntervalSeconds == 0 {
		c.Display.Rotate.IntervalSeconds = 30
	}
	if c.Display.Rotate.PauseSeconds == 0 {
		c.Display.Rotate.PauseSeconds = 60
	}
	if c.Display.Night.Mode == "" {
		c.Display.Night.Mode = "dim"
	}
	if c.Display.Night.Dim == 0 {
		c.Display.Night.Dim = 0.75
	}
	if c.Display.Night.WakeSeconds == 0 {
		c.Display.Night.WakeSeconds = 30
	}
	if c.Grid.Columns == 0 {
		c.Grid.Columns = 6
	}
	if c.Grid.Rows == 0 {
		c.Grid.Rows = 4
	}
	for i := range c.Pages {
		for j := range c.Pages[i].Widgets {
			w := &c.Pages[i].Widgets[j]
			if w.Pos.W == 0 {
				w.Pos.W = 1
			}
			if w.Pos.H == 0 {
				w.Pos.H = 1
			}
			if w.Settings == nil {
				w.Settings = map[string]any{}
			}
		}
	}
}

func (c *Config) validate() error {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }

	if c.Locale != "de" && c.Locale != "en" {
		add("locale: %q not supported (de, en)", c.Locale)
	}
	if c.Timezone != "" {
		if _, err := time.LoadLocation(c.Timezone); err != nil {
			add("timezone: %v", err)
		}
	}
	switch c.Theme.Mode {
	case "auto", "light", "dark":
	default:
		add("theme.mode: %q invalid (auto, light, dark)", c.Theme.Mode)
	}
	c.validateDisplay(add)
	if c.Grid.Columns < 1 || c.Grid.Rows < 1 {
		add("grid: columns and rows must be >= 1")
	}
	if len(c.Pages) == 0 {
		add("pages: at least one page is required")
	}

	pageIDs := map[string]bool{}
	widgetIDs := map[string]bool{}
	for _, p := range c.Pages {
		if p.ID == "" {
			add("pages: page without id")
		} else if pageIDs[p.ID] {
			add("pages: duplicate page id %q", p.ID)
		}
		pageIDs[p.ID] = true
		for _, w := range p.Widgets {
			where := fmt.Sprintf("page %q widget %q", p.ID, w.ID)
			if w.ID == "" {
				add("page %q: widget without id", p.ID)
				continue
			}
			if widgetIDs[w.ID] {
				add("%s: duplicate widget id", where)
			}
			widgetIDs[w.ID] = true
			if _, ok := widget.New(w.Type); !ok {
				add("%s: unknown type %q (known: %s)", where, w.Type, strings.Join(widget.Types(), ", "))
			}
			if err := widget.ValidateSettings(w.Type, w.Settings, c.Connections); err != nil {
				add("%s: %v", where, err)
			}
			if w.Connection != "" {
				if _, ok := c.Connections[w.Connection]; !ok {
					add("%s: unknown connection %q", where, w.Connection)
				}
			}
			if w.Pos.X < 0 || w.Pos.Y < 0 || w.Pos.W < 1 || w.Pos.H < 1 ||
				w.Pos.X+w.Pos.W > c.Grid.Columns || w.Pos.Y+w.Pos.H > c.Grid.Rows {
				add("%s: pos %+v does not fit into the %dx%d grid", where, w.Pos, c.Grid.Columns, c.Grid.Rows)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
