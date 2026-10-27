// Package clock is the built-in clock widget. Time itself is ticked by the
// frontend (no server traffic); the provider only validates the timezone and
// delivers the zone info.
package clock

import (
	"context"
	"fmt"
	"time"
	_ "time/tzdata" // timezones must work in minimal container images

	"github.com/pmcgn/lumina/internal/widget"
)

const Type = "core.clock"

func init() {
	widget.Register(Type, func() widget.Provider { return &Clock{} })
}

type Clock struct {
	tz  string
	loc *time.Location
}

// parseSettings decodes and validates the widget settings.
func parseSettings(m map[string]any) (Settings, *time.Location, error) {
	var s Settings
	if err := widget.DecodeSettings(m, &s); err != nil {
		return s, nil, err
	}
	loc := time.Local
	if s.Timezone != "" {
		l, err := time.LoadLocation(s.Timezone)
		if err != nil {
			return s, nil, fmt.Errorf("setting \"timezone\": %w", err)
		}
		loc = l
	}
	return s, loc, nil
}

func (c *Clock) ValidateSettings(m map[string]any, _ map[string]map[string]any) error {
	_, _, err := parseSettings(m)
	return err
}

func (c *Clock) Start(_ context.Context, env widget.Env) error {
	s, loc, err := parseSettings(env.Settings)
	if err != nil {
		return err
	}
	c.tz, c.loc = s.Timezone, loc
	return nil
}
func (c *Clock) Fetch(context.Context) (any, error) {
	_, offset := time.Now().In(c.loc).Zone()
	return Data{Timezone: c.tz, UTCOffsetSeconds: offset}, nil
}

func (c *Clock) Interval() time.Duration { return 15 * time.Minute }
