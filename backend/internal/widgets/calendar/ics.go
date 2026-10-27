package calendar

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/apognu/gocal"
)

// localMarker is an internal TZID given to floating times (no zone in the
// feed), so they resolve to the configured timezone: the library would use the
// process timezone, which in a container is UTC.
const localMarker = "X-LUMINA-LOCAL"

// rawEvent is one occurrence of an event, expanded from the iCal source.
type rawEvent struct {
	UID      string
	Title    string
	Location string
	Start    time.Time
	End      time.Time // exclusive
	AllDay   bool
}

// windowsZones maps the zone names Outlook/Exchange write ("W. Europe
// Standard Time") to IANA names. Only common ones; unknown names fall back to
// the configured timezone instead of failing the whole calendar.
var windowsZones = map[string]string{
	"W. Europe Standard Time":        "Europe/Berlin",
	"Central Europe Standard Time":   "Europe/Budapest",
	"Central European Standard Time": "Europe/Warsaw",
	"Romance Standard Time":          "Europe/Paris",
	"GMT Standard Time":              "Europe/London",
	"Greenwich Standard Time":        "Atlantic/Reykjavik",
	"E. Europe Standard Time":        "Europe/Chisinau",
	"FLE Standard Time":              "Europe/Kiev",
	"GTB Standard Time":              "Europe/Bucharest",
	"Russian Standard Time":          "Europe/Moscow",
	"Turkey Standard Time":           "Europe/Istanbul",
	"Eastern Standard Time":          "America/New_York",
	"Central Standard Time":          "America/Chicago",
	"Mountain Standard Time":         "America/Denver",
	"Pacific Standard Time":          "America/Los_Angeles",
	"Alaskan Standard Time":          "America/Anchorage",
	"Atlantic Standard Time":         "America/Halifax",
	"UTC":                            "UTC",
	"India Standard Time":            "Asia/Kolkata",
	"China Standard Time":            "Asia/Shanghai",
	"Tokyo Standard Time":            "Asia/Tokyo",
	"AUS Eastern Standard Time":      "Australia/Sydney",
	"New Zealand Standard Time":      "Pacific/Auckland",
	"South Africa Standard Time":     "Africa/Johannesburg",
	"Arab Standard Time":             "Asia/Riyadh",
	"Singapore Standard Time":        "Asia/Singapore",
	"Korea Standard Time":            "Asia/Seoul",
	"E. South America Standard Time": "America/Sao_Paulo",
	"Israel Standard Time":           "Asia/Jerusalem",
}

var (
	tzOnce    sync.Once
	fallbackM sync.Mutex
	fallback  = time.UTC // replaced per expand() call; see below
)

// installTZMapper registers a process-wide mapper (the library only offers a
// global hook). It resolves IANA and Windows names and falls back to the
// timezone of the parse in progress.
func installTZMapper() {
	tzOnce.Do(func() {
		gocal.SetTZMapper(func(name string) (*time.Location, error) {
			name = strings.Trim(name, `"`)
			if name == localMarker {
				fallbackM.Lock()
				defer fallbackM.Unlock()
				return fallback, nil
			}
			if iana, ok := windowsZones[name]; ok {
				name = iana
			}
			if loc, err := time.LoadLocation(name); err == nil {
				return loc, nil
			}
			fallbackM.Lock()
			defer fallbackM.Unlock()
			return fallback, nil
		})
	})
}

// expand returns all occurrences overlapping [from, to), sorted by start.
// Floating times and unknown zones are interpreted in loc.
func expand(data []byte, from, to time.Time, loc *time.Location) ([]rawEvent, error) {
	installTZMapper()
	// The library's zone mapper is global, so parsing is serialized and the
	// fallback zone is set while holding the lock.
	parseMu.Lock()
	defer parseMu.Unlock()
	fallbackM.Lock()
	fallback = loc
	fallbackM.Unlock()

	p := gocal.NewParser(bytes.NewReader(normalize(data)))
	p.Start, p.End = &from, &to
	p.AllDayEventsTZ = loc
	p.Strict.Mode = gocal.StrictModeFailEvent      // a broken event must not take the whole calendar down
	p.Duplicate.Mode = gocal.DuplicateModeKeepLast // tolerate repeated properties
	if err := p.Parse(); err != nil {
		return nil, fmt.Errorf("invalid iCal data: %w", err)
	}

	var out []rawEvent
	for _, e := range p.Events {
		if e.Start == nil || strings.EqualFold(e.Status, "CANCELLED") {
			continue
		}
		ev := rawEvent{UID: e.Uid, Title: e.Summary, Location: e.Location, Start: *e.Start}
		ev.AllDay = e.RawStart.Params["VALUE"] == "DATE"
		switch {
		case e.End != nil && ev.AllDay:
			// The library reports the end of an all-day event as DTEND minus 1 ms
			// (an inclusive end); iCal's DTEND is the exclusive next midnight.
			ev.End = e.End.Add(time.Millisecond)
		case e.End != nil:
			ev.End = *e.End
		case e.Duration != nil:
			ev.End = ev.Start.Add(*e.Duration)
		case ev.AllDay:
			ev.End = ev.Start.AddDate(0, 0, 1)
		default:
			ev.End = ev.Start
		}
		if ev.AllDay {
			ev.Start = dayStart(ev.Start, loc)
			ev.End = dayStart(ev.End, loc)
			if !ev.End.After(ev.Start) {
				ev.End = ev.Start.AddDate(0, 0, 1)
			}
		}
		// keep what overlaps the window (zero-length events at their start)
		if ev.End.Before(from) || !ev.Start.Before(to) || (ev.End.Equal(from) && !ev.End.Equal(ev.Start)) {
			continue
		}
		if strings.TrimSpace(ev.Title) == "" {
			ev.Title = "(ohne Titel)"
		}
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].AllDay && !out[j].AllDay
	})
	return out, nil
}

var parseMu sync.Mutex

// dayStart returns the local midnight of t's calendar date in loc, using the
// date components of t as written (all-day dates are not instants).
func dayStart(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// floatingProps are the properties whose date-time values may be "floating".
var floatingProps = map[string]bool{"DTSTART": true, "DTEND": true, "EXDATE": true, "RDATE": true, "RECURRENCE-ID": true}

// normalize prepares feed text for the parser:
//   - adds the DTSTAMP property that RFC 5545 requires but some feeds omit and
//     the parser insists on;
//   - tags floating date-times (no Z, no TZID) with the internal zone marker.
func normalize(data []byte) []byte {
	var out bytes.Buffer
	inEvent, seen := false, false
	for _, line := range bytes.Split(data, []byte("\n")) {
		text := strings.TrimRight(string(line), "\r")
		upper := strings.ToUpper(text)
		switch {
		case upper == "BEGIN:VEVENT":
			inEvent, seen = true, false
		case inEvent && (strings.HasPrefix(upper, "DTSTAMP:") || strings.HasPrefix(upper, "DTSTAMP;")):
			seen = true
		case inEvent && upper == "END:VEVENT":
			if !seen {
				out.WriteString("DTSTAMP:19700101T000000Z\r\n")
			}
			inEvent = false
		case inEvent:
			if tagged, ok := tagFloating(text, upper); ok {
				out.WriteString(tagged)
				out.WriteString("\r\n")
				continue
			}
		}
		out.Write(line)
		out.WriteString("\n")
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n"))
}

// tagFloating adds the zone marker to a property line with a floating
// date-time value.
func tagFloating(text, upper string) (string, bool) {
	colon := strings.Index(text, ":")
	if colon < 0 {
		return "", false
	}
	head := upper[:colon]
	name := head
	if i := strings.Index(head, ";"); i >= 0 {
		name = head[:i]
	}
	if !floatingProps[name] {
		return "", false
	}
	if strings.Contains(head, "TZID=") || strings.Contains(head, "VALUE=DATE") && !strings.Contains(head, "VALUE=DATE-TIME") {
		return "", false
	}
	value := strings.TrimSpace(text[colon+1:])
	if value == "" || strings.Contains(value, "Z") || !strings.Contains(value, "T") {
		return "", false
	}
	return text[:colon] + ";TZID=" + localMarker + text[colon:], true
}
