package calendar

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var berlin = mustLoc("Europe/Berlin")

func mustLoc(n string) *time.Location {
	l, err := time.LoadLocation(n)
	if err != nil {
		panic(err)
	}
	return l
}

// Window used by most tests: Sat 17 Oct 2026 00:00 to Sun 1 Nov 2026 00:00 (Berlin).
var (
	winFrom = time.Date(2026, 10, 17, 0, 0, 0, 0, berlin)
	winTo   = time.Date(2026, 11, 1, 0, 0, 0, 0, berlin)
)

func cal(events ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" + strings.Join(events, "") + "END:VCALENDAR\r\n")
}

func ev(lines ...string) string {
	return "BEGIN:VEVENT\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VEVENT\r\n"
}

func run(t *testing.T, data []byte) []rawEvent {
	t.Helper()
	out, err := expand(data, winFrom, winTo, berlin)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func local(e rawEvent) string {
	if e.AllDay {
		return fmt.Sprintf("%s %s..%s allday", e.Title, e.Start.Format("2006-01-02"), e.End.Format("2006-01-02"))
	}
	return fmt.Sprintf("%s %s..%s", e.Title, e.Start.In(berlin).Format("01-02 15:04"), e.End.In(berlin).Format("01-02 15:04"))
}

func list(es []rawEvent) []string {
	var out []string
	for _, e := range es {
		out = append(out, local(e))
	}
	return out
}

func expectEvents(t *testing.T, got []rawEvent, want ...string) {
	t.Helper()
	g := list(got)
	if strings.Join(g, "\n") != strings.Join(want, "\n") {
		t.Errorf("events:\n got:\n  %s\nwant:\n  %s", strings.Join(g, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestTimeZones(t *testing.T) {
	got := run(t, cal(
		ev("UID:a", "SUMMARY:Berlin TZID", "DTSTART;TZID=Europe/Berlin:20261020T090000", "DTEND;TZID=Europe/Berlin:20261020T100000"),
		ev("UID:b", "SUMMARY:UTC", "DTSTART:20261021T070000Z", "DTEND:20261021T080000Z"), // 09:00 Berlin (CEST)
		ev("UID:c", "SUMMARY:Floating", "DTSTART:20261022T140000", "DTEND:20261022T150000"),
		ev("UID:d", "SUMMARY:New York", "DTSTART;TZID=America/New_York:20261023T090000", "DTEND;TZID=America/New_York:20261023T100000"), // 15:00 Berlin
		ev("UID:e", "SUMMARY:Windows zone", "DTSTART;TZID=W. Europe Standard Time:20261024T110000", "DTEND;TZID=W. Europe Standard Time:20261024T120000"),
		ev("UID:f", "SUMMARY:Unknown zone", "DTSTART;TZID=Mars/Olympus:20261026T080000", "DTEND;TZID=Mars/Olympus:20261026T090000"),
	))
	expectEvents(t, got,
		"Berlin TZID 10-20 09:00..10-20 10:00",
		"UTC 10-21 09:00..10-21 10:00",
		"Floating 10-22 14:00..10-22 15:00",
		"New York 10-23 15:00..10-23 16:00",
		"Windows zone 10-24 11:00..10-24 12:00",
		"Unknown zone 10-26 08:00..10-26 09:00", // falls back to the configured zone instead of failing
	)
}

func TestAllDayEvents(t *testing.T) {
	got := run(t, cal(
		ev("UID:a", "SUMMARY:Single", "DTSTART;VALUE=DATE:20261020", "DTEND;VALUE=DATE:20261021"),
		ev("UID:b", "SUMMARY:No end", "DTSTART;VALUE=DATE:20261021"),
		ev("UID:c", "SUMMARY:Holiday", "DTSTART;VALUE=DATE:20261023", "DTEND;VALUE=DATE:20261026"), // Fri-Sun
		ev("UID:d", "SUMMARY:Timed same day", "DTSTART;TZID=Europe/Berlin:20261020T180000", "DTEND;TZID=Europe/Berlin:20261020T190000"),
	))
	// All-day first on a day, then timed; end is exclusive.
	expectEvents(t, got,
		"Single 2026-10-20..2026-10-21 allday",
		"Timed same day 10-20 18:00..10-20 19:00",
		"No end 2026-10-21..2026-10-22 allday",
		"Holiday 2026-10-23..2026-10-26 allday",
	)
	if !got[0].AllDay || got[0].Start.Location() != berlin {
		t.Errorf("all-day start must be local midnight in the configured zone: %v", got[0].Start)
	}
}

func TestDurationInsteadOfEnd(t *testing.T) {
	got := run(t, cal(ev("UID:a", "SUMMARY:Dur", "DTSTART;TZID=Europe/Berlin:20261020T090000", "DURATION:PT90M")))
	expectEvents(t, got, "Dur 10-20 09:00..10-20 10:30")
}

func TestWeeklyRecurrenceWithExceptionAndOverride(t *testing.T) {
	// Mon/Wed 18:00-19:00 with an exception on Wed 21 Oct and the Mon 26 Oct instance moved to Tue 27 Oct 17:00.
	got := run(t, cal(
		ev("UID:r1", "SUMMARY:Training", "DTSTART;TZID=Europe/Berlin:20261012T180000", "DTEND;TZID=Europe/Berlin:20261012T190000",
			"RRULE:FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20261031T000000Z", "EXDATE;TZID=Europe/Berlin:20261021T180000"),
		ev("UID:r1", "SUMMARY:Training (verschoben)", "RECURRENCE-ID;TZID=Europe/Berlin:20261026T180000",
			"DTSTART;TZID=Europe/Berlin:20261027T170000", "DTEND;TZID=Europe/Berlin:20261027T180000"),
	))
	expectEvents(t, got,
		"Training 10-19 18:00..10-19 19:00",
		// Wed 21 is excluded; Mon 26 was moved to Tue 27 17:00; Wed 28 is regular.
		"Training (verschoben) 10-27 17:00..10-27 18:00",
		"Training 10-28 18:00..10-28 19:00",
	)
}

func TestRecurrenceStaysAtLocalTimeAcrossDST(t *testing.T) {
	// DST ends on Sun 25 Oct 2026 in Europe. A daily 09:00 Berlin event must stay 09:00 local.
	got := run(t, cal(ev("UID:d", "SUMMARY:Daily", "DTSTART;TZID=Europe/Berlin:20261023T090000", "DTEND;TZID=Europe/Berlin:20261023T093000", "RRULE:FREQ=DAILY;COUNT=5")))
	expectEvents(t, got,
		"Daily 10-23 09:00..10-23 09:30",
		"Daily 10-24 09:00..10-24 09:30",
		"Daily 10-25 09:00..10-25 09:30",
		"Daily 10-26 09:00..10-26 09:30",
		"Daily 10-27 09:00..10-27 09:30",
	)
}

func TestYearlyBirthdayAndCountAndInfinite(t *testing.T) {
	got := run(t, cal(
		ev("UID:b", "SUMMARY:Geburtstag Anna", "DTSTART;VALUE=DATE:19900420", "DTEND;VALUE=DATE:19900421", "RRULE:FREQ=YEARLY"), // April: not in window
		ev("UID:b2", "SUMMARY:Geburtstag Ben", "DTSTART;VALUE=DATE:19851028", "DTEND;VALUE=DATE:19851029", "RRULE:FREQ=YEARLY"),
		ev("UID:c", "SUMMARY:Limited", "DTSTART;TZID=Europe/Berlin:20261001T100000", "DTEND;TZID=Europe/Berlin:20261001T110000", "RRULE:FREQ=WEEKLY;COUNT=3"), // 1, 8, 15 Oct: all before window
		ev("UID:i", "SUMMARY:Forever", "DTSTART;TZID=Europe/Berlin:20200101T060000", "DTEND;TZID=Europe/Berlin:20200101T061500", "RRULE:FREQ=DAILY"),
	))
	var titles []string
	forever := 0
	for _, e := range got {
		if e.Title == "Forever" {
			forever++
			continue
		}
		titles = append(titles, local(e))
	}
	if strings.Join(titles, "|") != "Geburtstag Ben 2026-10-28..2026-10-29 allday" {
		t.Errorf("got %v", titles)
	}
	if forever != 15 { // one per day, 17 Oct .. 31 Oct
		t.Errorf("infinite daily recurrence: %d occurrences in the 15-day window, want 15", forever)
	}
}

func TestCancelledAndEscapingAndFolding(t *testing.T) {
	got := run(t, cal(
		ev("UID:x", "SUMMARY:Abgesagt", "STATUS:CANCELLED", "DTSTART;TZID=Europe/Berlin:20261020T090000", "DTEND;TZID=Europe/Berlin:20261020T100000"),
		ev("UID:y", `SUMMARY:Treffen\, Büro\; Raum 2`, "LOCATION:Hauptstr. 1\\, Köln",
			"DTSTART;TZID=Europe/Berlin:20261021T090000", "DTEND;TZID=Europe/Berlin:20261021T100000"),
		ev("UID:z", "SUMMARY:Ein sehr langer Titel der in der iCal-Datei über mehrere ", " Zeilen umgebrochen wurde", "DTSTART;TZID=Europe/Berlin:20261022T090000", "DTEND;TZID=Europe/Berlin:20261022T100000"),
	))
	if len(got) != 2 {
		t.Fatalf("cancelled events must be dropped: %v", list(got))
	}
	if got[0].Title != "Treffen, Büro; Raum 2" || got[0].Location != "Hauptstr. 1, Köln" {
		t.Errorf("escapes: %q / %q", got[0].Title, got[0].Location)
	}
	if !strings.Contains(got[1].Title, "mehrere Zeilen") {
		t.Errorf("line folding: %q", got[1].Title)
	}
}

func TestWindowEdges(t *testing.T) {
	got := run(t, cal(
		ev("UID:a", "SUMMARY:Before", "DTSTART;TZID=Europe/Berlin:20261016T100000", "DTEND;TZID=Europe/Berlin:20261016T110000"),
		ev("UID:b", "SUMMARY:Overlaps start", "DTSTART;TZID=Europe/Berlin:20261016T230000", "DTEND;TZID=Europe/Berlin:20261017T020000"),
		ev("UID:c", "SUMMARY:Ends exactly at start", "DTSTART;TZID=Europe/Berlin:20261016T220000", "DTEND;TZID=Europe/Berlin:20261017T000000"),
		ev("UID:d", "SUMMARY:Last day", "DTSTART;TZID=Europe/Berlin:20261031T230000", "DTEND;TZID=Europe/Berlin:20261101T010000"),
		ev("UID:e", "SUMMARY:After", "DTSTART;TZID=Europe/Berlin:20261101T000000", "DTEND;TZID=Europe/Berlin:20261101T010000"),
		ev("UID:f", "SUMMARY:All-day multi spanning start", "DTSTART;VALUE=DATE:20261015", "DTEND;VALUE=DATE:20261018"),
	))
	expectEvents(t, got,
		"All-day multi spanning start 2026-10-15..2026-10-18 allday",
		"Overlaps start 10-16 23:00..10-17 02:00",
		"Last day 10-31 23:00..11-01 01:00",
	)
}

func TestBrokenInput(t *testing.T) {
	// Garbage must not panic; an event with a broken date is skipped, not fatal for the others.
	if _, err := expand([]byte("this is not a calendar"), winFrom, winTo, berlin); err != nil {
		t.Logf("garbage -> %v (acceptable)", err)
	}
	got, err := expand(cal(
		ev("UID:ok", "SUMMARY:Fine", "DTSTART;TZID=Europe/Berlin:20261020T090000", "DTEND;TZID=Europe/Berlin:20261020T100000"),
	), winFrom, winTo, berlin)
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if got, err := expand(cal(ev("UID:n", "DTSTART;TZID=Europe/Berlin:20261020T090000", "DTEND;TZID=Europe/Berlin:20261020T100000")), winFrom, winTo, berlin); err != nil || len(got) != 1 || got[0].Title == "" {
		t.Errorf("event without title must get a placeholder: %v %v", got, err)
	}
}
