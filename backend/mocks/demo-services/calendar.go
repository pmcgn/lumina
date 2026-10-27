// Demo calendars: iCal feeds generated on every request, with dates relative to
// the current time, so there is always something running, upcoming and overdue.
package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func registerCalendars(mux *http.ServeMux) {
	mux.HandleFunc("/calendar/family.ics", func(w http.ResponseWriter, _ *http.Request) { serveICS(w, "Familie", familyEvents(time.Now())) })
	mux.HandleFunc("/calendar/work.ics", func(w http.ResponseWriter, _ *http.Request) { serveICS(w, "Arbeit", workEvents(time.Now())) })
}

func serveICS(w http.ResponseWriter, name string, events []string) {
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	lines := append([]string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//lumina demo//EN", "X-WR-CALNAME:" + name}, events...)
	lines = append(lines, "END:VCALENDAR")
	fmt.Fprint(w, strings.Join(lines, "\r\n")+"\r\n")
}

type icsEvent struct {
	uid, title, location string
	start, end           time.Time
	allDay               bool
	rrule                string
	cancelled            bool
}

func (e icsEvent) String() string {
	lines := []string{"BEGIN:VEVENT", "UID:" + e.uid + "@lumina-demo", "DTSTAMP:20260101T000000Z", "SUMMARY:" + e.title}
	if e.allDay {
		lines = append(lines, "DTSTART;VALUE=DATE:"+e.start.Format("20060102"), "DTEND;VALUE=DATE:"+e.end.Format("20060102"))
	} else {
		lines = append(lines, "DTSTART:"+e.start.UTC().Format("20060102T150405Z"), "DTEND:"+e.end.UTC().Format("20060102T150405Z"))
	}
	if e.rrule != "" {
		lines = append(lines, "RRULE:"+e.rrule)
	}
	if e.location != "" {
		lines = append(lines, "LOCATION:"+e.location)
	}
	if e.cancelled {
		lines = append(lines, "STATUS:CANCELLED")
	}
	return strings.Join(append(lines, "END:VEVENT"), "\r\n")
}

func familyEvents(now time.Time) []string {
	day := func(k int) time.Time { // local midnight, k days from today
		y, m, d := now.Date()
		return time.Date(y, m, d+k, 0, 0, 0, 0, now.Location())
	}
	at := func(k, h, m int) time.Time {
		return day(k).Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
	}
	es := []icsEvent{
		{uid: "f1", title: "Elternabend Grundschule", location: "Aula, Schillerstraße 4", start: now.Add(-30 * time.Minute), end: now.Add(40 * time.Minute)},
		{uid: "f2", title: "Telefonat mit Oma", start: now.Add(90 * time.Minute), end: now.Add(2 * time.Hour)},
		{uid: "f3", title: "Gelbe Tonne rausstellen", allDay: true, start: day(0), end: day(1)},
		{uid: "f4", title: "Brunch bei Jana", location: "Café Central, Köln-Ehrenfeld", start: at(1, 10, 0), end: at(1, 11, 30)},
		{uid: "f5", title: "Fußballtraining Milian", location: "Sportplatz Nord", start: at(1, 17, 30), end: at(1, 19, 0), rrule: "FREQ=WEEKLY;BYDAY=TU,TH"},
		{uid: "f6", title: "Geburtstag Oma", allDay: true, start: day(3), end: day(4), rrule: "FREQ=YEARLY"},
		{uid: "f7", title: "Zahnarzt Dr. Weber", start: at(4, 8, 15), end: at(4, 9, 0)},
		{uid: "f8", title: "Urlaub Ostsee", allDay: true, start: day(6), end: day(11)},
		{uid: "f9", title: "Abgesagter Termin", start: at(2, 12, 0), end: at(2, 13, 0), cancelled: true},
	}
	return stringify(es)
}

func workEvents(now time.Time) []string {
	day := func(k int) time.Time {
		y, m, d := now.Date()
		return time.Date(y, m, d+k, 0, 0, 0, 0, now.Location())
	}
	at := func(k, h, m int) time.Time {
		return day(k).Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
	}
	es := []icsEvent{
		{uid: "w1", title: "Team-Standup", start: at(1, 9, 0), end: at(1, 9, 15), rrule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"},
		{uid: "w2", title: "Review mit Sandra: Quartalsplanung und Budget für das nächste Jahr", location: "Raum 3.14", start: at(2, 14, 0), end: at(2, 15, 0)},
		{uid: "w3", title: "Release 2.0 Go-Live", start: at(5, 16, 0), end: at(5, 18, 0)},
	}
	return stringify(es)
}

func stringify(es []icsEvent) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.String()
	}
	return out
}
