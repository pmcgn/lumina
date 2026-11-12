package config

import "github.com/pmcgn/lumina/internal/backgrounds"

// Wire model: the configuration the browser receives (message "config").
// This file is converted to TypeScript by tygo (frontend/src/lib/config.gen.ts).
// Connections and secrets are never part of it. Keep server-only settings in config.go.

// ClientConfig is what the browser receives.
type ClientConfig struct {
	Locale  Locale        `json:"locale"`
	Theme   ClientTheme   `json:"theme"`
	Grid    Grid          `json:"grid"`
	Display DisplayClient `json:"display"`
	Pages   []Page        `json:"pages"`
}

// Client returns the browser-facing part of the configuration.
func (c *Config) Client() ClientConfig {
	return ClientConfig{Locale: c.Locale, Theme: c.Theme.Client(), Grid: c.Grid, Display: c.Display.Client(), Pages: c.Pages}
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
	Mode       ThemeMode  `yaml:"mode"`
	Accent     string     `yaml:"accent"` // CSS color, optional
	Background Background `yaml:"background"`
	// WidgetOpacity is the opacity of the widget cards, 0.1..1 (default 1 = opaque).
	// Below 1 the background picture shows through the cards.
	WidgetOpacity *float64 `yaml:"widgetOpacity"`
}

// Background configures the picture behind the widgets, one per color scheme.
// A value is the name of an embedded picture (file name without extension) or a
// file path; empty means no picture.
type Background struct {
	Light string `yaml:"light"`
	Dark  string `yaml:"dark"`
}

// ClientTheme is the theme as the browser receives it.
type ClientTheme struct {
	Mode       ThemeMode        `json:"mode"`
	Accent     string           `json:"accent"`
	Background BackgroundClient `json:"background"`
	// WidgetOpacity is the card opacity, 0.1..1.
	WidgetOpacity float64 `json:"widgetOpacity"`
}

// BackgroundClient holds the URLs of the background pictures ("" = none). The
// server serves them under these URLs; the paths in the configuration never reach the browser.
type BackgroundClient struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

// BackgroundPath is the URL prefix under which the server serves the pictures
// (BackgroundPath + "light" or "dark").
const BackgroundPath = "/backgrounds/"

// Client returns the browser-facing theme.
func (t Theme) Client() ClientTheme {
	url := func(variant, value string) string {
		if value == "" {
			return ""
		}
		return BackgroundPath + variant + "?v=" + backgrounds.Version(value)
	}
	opacity := 1.0
	if t.WidgetOpacity != nil {
		opacity = *t.WidgetOpacity
	}
	return ClientTheme{
		Mode:          t.Mode,
		Accent:        t.Accent,
		WidgetOpacity: opacity,
		Background: BackgroundClient{
			Light: url("light", t.Background.Light),
			Dark:  url("dark", t.Background.Dark),
		},
	}
}

type Grid struct {
	Columns int `yaml:"columns" json:"columns"`
	Rows    int `yaml:"rows" json:"rows"`
}

type Page struct {
	ID      string   `yaml:"id" json:"id"`
	Title   string   `yaml:"title" json:"title"`
	Home    bool     `yaml:"home" json:"home"` // at most one page; the UI returns to it after inactivity
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

// TransitionType is the animation between two pages.
type TransitionType string

const (
	TransitionTypeFade      TransitionType = "fade"      // new page fades in (default)
	TransitionTypeFly       TransitionType = "fly"       // pages fly sideways in the direction of the tab order
	TransitionTypeSlide     TransitionType = "slide"     // the old page collapses sideways while the new one unfolds
	TransitionTypeBlur      TransitionType = "blur"      // crossfade with a blur
	TransitionTypeCrossfade TransitionType = "crossfade" // old page fades out while the new one fades in
	TransitionTypeScale     TransitionType = "scale"     // crossfade with a slight zoom
)

// PageTransition configures the page change animation.
type PageTransition struct {
	Type       TransitionType `yaml:"type" json:"type"`             // default fade
	DurationMs int            `yaml:"durationMs" json:"durationMs"` // 50..2000, default 300
}

// NightMode is how the screen looks during the night window.
type NightMode string

const (
	NightModeDim   NightMode = "dim"   // darken the dashboard
	NightModeClock NightMode = "clock" // only a clock; a touch wakes the dashboard
)

// DisplayClient is the resolved display behavior sent to the browser.
type DisplayClient struct {
	Rotate     Rotate         `json:"rotate"`
	Night      NightClient    `json:"night"`
	SoftReload string         `json:"softReload"` // HH:MM, "" = off
	Lite       bool           `json:"lite"`
	Transition PageTransition `json:"transition"`
}

type NightClient struct {
	Enabled     bool      `json:"enabled"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	Mode        NightMode `json:"mode"`
	Dim         float64   `json:"dim"`
	WakeSeconds int       `json:"wakeSeconds"`
}
