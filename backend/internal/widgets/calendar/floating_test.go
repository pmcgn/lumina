package calendar

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFloatingTimesUseTheConfiguredZoneNotTheMachinesZone(t *testing.T) {
	// Run the same feed with configured zones that differ from the machine's.
	data := cal(
		ev("UID:f", "SUMMARY:Floating", "DTSTART:20261022T140000", "DTEND:20261022T150000"),
		ev("UID:g", "SUMMARY:Floating weekly", "DTSTART:20261019T080000", "DTEND:20261019T090000", "RRULE:FREQ=WEEKLY;COUNT=2", "EXDATE:20261026T080000"),
		ev("UID:h", "SUMMARY:Param but floating", "DTSTART;X-FOO=bar:20261023T100000", "DTEND;X-FOO=bar:20261023T110000"),
	)
	for _, name := range []string{"Europe/Berlin", "America/New_York", "Asia/Tokyo", "UTC"} {
		loc := mustLoc(name)
		got, err := expand(data, time.Date(2026, 10, 17, 0, 0, 0, 0, loc), time.Date(2026, 11, 1, 0, 0, 0, 0, loc), loc)
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for _, e := range got {
			lines = append(lines, fmt.Sprintf("%s %s", e.Title, e.Start.In(loc).Format("01-02 15:04")))
		}
		// the 26th is excluded by EXDATE
		want := "Floating weekly 10-19 08:00|Floating 10-22 14:00|Param but floating 10-23 10:00"
		if strings.Join(lines, "|") != want {
			t.Errorf("%s:\n got  %s\n want %s", name, strings.Join(lines, "|"), want)
		}
	}
}

func TestNormalizeLeavesZonedAndAllDayValuesAlone(t *testing.T) {
	in := strings.Join([]string{
		"BEGIN:VEVENT",
		"DTSTART;TZID=Europe/Berlin:20261020T090000",
		"DTEND:20261020T080000Z",
		"EXDATE;VALUE=DATE:20261021",
		"DTSTART;VALUE=DATE:20261022",
		"SUMMARY:DTSTART:20261020T090000",
		"DTSTAMP:20260101T000000Z",
		"END:VEVENT",
		"",
	}, "\r\n")
	out := string(normalize([]byte(in)))
	if strings.Contains(out, localMarker) {
		t.Errorf("only floating date-times may be tagged:\n%s", out)
	}
	if strings.Count(out, "DTSTAMP") != 1 {
		t.Errorf("an existing DTSTAMP must not be duplicated:\n%s", out)
	}
}
