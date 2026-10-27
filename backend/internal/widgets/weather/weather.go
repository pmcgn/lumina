// Package weather is the weather widget. The data source is Open-Meteo (free,
// no API key); upstream data is mapped to Lumina's own model, with
// condition codes instead of upstream texts so the frontend can translate them.
package weather

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/pmcgn/lumina/internal/httpx"
	"github.com/pmcgn/lumina/internal/widget"
)

const Type = "weather.forecast"

const (
	defaultForecastURL  = "https://api.open-meteo.com/v1/forecast"
	defaultGeocodingURL = "https://geocoding-api.open-meteo.com/v1/search"
	hourlyCount         = 24
	dailyCount          = 7
)

func init() {
	widget.Register(Type, func() widget.Provider { return newProvider() })
}

type Provider struct {
	forecastURL  string
	geocodingURL string
	client       *http.Client
	now          func() time.Time

	name     string // display name
	location string // geocoding query, if no coordinates are configured

	mu       sync.Mutex
	lat, lon float64
	resolved bool
}

func newProvider() *Provider {
	return &Provider{forecastURL: defaultForecastURL, geocodingURL: defaultGeocodingURL, client: httpx.NewClient(), now: time.Now}
}

// parseSettings decodes and validates the widget settings.
func parseSettings(m map[string]any) (Settings, error) {
	var s Settings
	if err := widget.DecodeSettings(m, &s); err != nil {
		return s, err
	}
	switch {
	case (s.Latitude == nil) != (s.Longitude == nil):
		return s, errors.New("settings: latitude and longitude must be set together")
	case s.Latitude != nil:
		if *s.Latitude < -90 || *s.Latitude > 90 || *s.Longitude < -180 || *s.Longitude > 180 {
			return s, errors.New("settings: latitude/longitude out of range")
		}
	case s.Location == "":
		return s, errors.New("settings: set either latitude and longitude, or location")
	}
	return s, nil
}

func (p *Provider) ValidateSettings(m map[string]any, _ map[string]map[string]any) error {
	_, err := parseSettings(m)
	return err
}

func (p *Provider) Start(_ context.Context, env widget.Env) error {
	s, err := parseSettings(env.Settings)
	if err != nil {
		return err
	}
	p.location = s.Location
	p.name = s.Label
	if p.name == "" {
		p.name = s.Location
	}
	if s.Latitude != nil {
		p.lat, p.lon, p.resolved = *s.Latitude, *s.Longitude, true
	}
	// A location name is resolved on the first fetch (keeps Start free of network I/O).
	return nil
}

func (p *Provider) Interval() time.Duration { return 15 * time.Minute }

func (p *Provider) Fetch(ctx context.Context) (any, error) {
	lat, lon, err := p.coordinates(ctx)
	if err != nil {
		return nil, err
	}
	var r response
	q := url.Values{
		"latitude":        {fmt.Sprint(lat)},
		"longitude":       {fmt.Sprint(lon)},
		"current":         {"temperature_2m,apparent_temperature,relative_humidity_2m,precipitation,weather_code,wind_speed_10m,wind_direction_10m,is_day"},
		"hourly":          {"temperature_2m,precipitation_probability,weather_code"},
		"daily":           {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max,sunrise,sunset"},
		"timezone":        {"auto"}, // daily aggregates are for local days
		"timeformat":      {"unixtime"},
		"forecast_days":   {fmt.Sprint(dailyCount)},
		"wind_speed_unit": {"kmh"},
	}
	if err := httpx.GetJSON(ctx, p.client, p.forecastURL+"?"+q.Encode(), &r); err != nil {
		return nil, err
	}
	return r.toData(p.name, p.now())
}

// coordinates returns the configured or geocoded position (cached after the first success).
func (p *Provider) coordinates(ctx context.Context) (lat, lon float64, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.resolved {
		return p.lat, p.lon, nil
	}
	var g struct {
		Results []struct {
			Name      string  `json:"name"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	q := url.Values{"name": {p.location}, "count": {"1"}, "format": {"json"}}
	if err := httpx.GetJSON(ctx, p.client, p.geocodingURL+"?"+q.Encode(), &g); err != nil {
		return 0, 0, fmt.Errorf("geocoding: %w", err)
	}
	if len(g.Results) == 0 {
		return 0, 0, fmt.Errorf("geocoding: location %q not found", p.location)
	}
	res := g.Results[0]
	p.lat, p.lon, p.resolved = res.Latitude, res.Longitude, true
	if p.name == "" {
		p.name = res.Name
	}
	return p.lat, p.lon, nil
}
