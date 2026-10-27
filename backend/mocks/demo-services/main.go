// demo-services provides stand-ins for the systems the dashboard integrates, so
// the whole dashboard can be tried without secrets and without touching real
// installations. One process serves all of them on one port:
//
// openHAB under /rest/ (sitemap "Demo", switchable devices, live events);
// Vikunja under /api/v1/ (token "demo", tasks relative to now: complete, undo,
// add); two iCal feeds under /calendar/ (family.ics, work.ics) with dates
// relative to now; and Grafana under /api/ (token "demo", dashboard "demo-haus"
// with generated household curves).
//
// Run it with "go run ./mocks/demo-services -addr :18080". Weather has no
// stand-in: the Open-Meteo API is public and needs no key.
package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":18080", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	newOpenHAB().register(mux)
	newVikunja().register(mux)
	registerCalendars(mux)
	registerGrafana(mux)

	log.Printf("demo services on %s (openHAB sitemap Demo, Vikunja token %q, /calendar/family.ics, /calendar/work.ics)", *addr, demoToken)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
