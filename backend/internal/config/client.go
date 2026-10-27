package config

// Wire model: the configuration the browser receives (message "config").
// This file is converted to TypeScript by tygo (frontend/src/lib/config.gen.ts).
// Connections and secrets are never part of it. Keep server-only settings in config.go.

// ClientConfig is what the browser receives.
type ClientConfig struct {
	Locale  Locale        `json:"locale"`
	Theme   Theme         `json:"theme"`
	Grid    Grid          `json:"grid"`
	Display DisplayClient `json:"display"`
	Pages   []Page        `json:"pages"`
}

// Client returns the browser-facing part of the configuration.
func (c *Config) Client() ClientConfig {
	return ClientConfig{Locale: c.Locale, Theme: c.Theme, Grid: c.Grid, Display: c.Display.Client(), Pages: c.Pages}
}

// Locale is the UI language.
type Locale string

const (
	LocaleDE Locale = "de"
	LocaleEN Locale = "en"
)

// ThemeMode selects the color scheme; auto follows the device.
type ThemeMode string

const (
	ThemeModeAuto  ThemeMode = "auto"
	ThemeModeLight ThemeMode = "light"
	ThemeModeDark  ThemeMode = "dark"
)

type Theme struct {
	Mode   ThemeMode `yaml:"mode" json:"mode"`
	Accent string    `yaml:"accent" json:"accent"` // CSS color, optional
}

type Grid struct {
	Columns int `yaml:"columns" json:"columns"`
	Rows    int `yaml:"rows" json:"rows"`
}

type Page struct {
	ID      string   `yaml:"id" json:"id"`
	Title   string   `yaml:"title" json:"title"`
	Widgets []Widget `yaml:"widgets" json:"widgets"`
}

type Pos struct {
	X int `yaml:"x" json:"x"`
	Y int `yaml:"y" json:"y"`
	W int `yaml:"w" json:"w"`
	H int `yaml:"h" json:"h"`
}

// Widget is one widget instance on a page. The connection name and the
// alwaysOn flag are server-side only and not sent.
type Widget struct {
	ID         string         `yaml:"id" json:"id"`
	Type       string         `yaml:"type" json:"type"`
	Connection string         `yaml:"connection" json:"-"`
	Pos        Pos            `yaml:"pos" json:"pos"`
	ReadOnly   bool           `yaml:"readOnly" json:"readOnly"`
	AlwaysOn   bool           `yaml:"alwaysOn" json:"-"`
	Settings   map[string]any `yaml:"settings" json:"settings"`
}

// Rotate cycles through the pages automatically.
type Rotate struct {
	Enabled         bool `yaml:"enabled" json:"enabled"`
	IntervalSeconds int  `yaml:"intervalSeconds" json:"intervalSeconds"` // time per page, default 30
	PauseSeconds    int  `yaml:"pauseSeconds" json:"pauseSeconds"`       // pause after a touch, default 60
}

// NightMode is how the screen looks during the night window.
type NightMode string

const (
	NightModeDim   NightMode = "dim"   // darken the dashboard
	NightModeClock NightMode = "clock" // only a clock; a touch wakes the dashboard
)

// DisplayClient is the resolved display behavior sent to the browser.
type DisplayClient struct {
	Rotate     Rotate      `json:"rotate"`
	Night      NightClient `json:"night"`
	SoftReload string      `json:"softReload"` // HH:MM, "" = off
	Lite       bool        `json:"lite"`
}

type NightClient struct {
	Enabled     bool      `json:"enabled"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	Mode        NightMode `json:"mode"`
	Dim         float64   `json:"dim"`
	WakeSeconds int       `json:"wakeSeconds"`
}
