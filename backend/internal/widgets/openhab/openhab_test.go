package openhab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/sse"
	"github.com/pmcgn/lumina/internal/widget"
)

// --- fake openHAB ---------------------------------------------------------

type fakeOH struct {
	mu       sync.Mutex
	states   map[string]string
	commands []string // "item=CMD"
	pageGets int
	events   chan string // raw SSE blocks to send
	ctype    string      // Content-Type of the last command
	srv      *httptest.Server
}

func newFakeOH(t *testing.T) *fakeOH {
	f := &fakeOH{
		states: map[string]string{"Light": "OFF", "Amb": "0,0,0", "Mode": "AUTO", "Locked": "OFF", "Power": "12.5", "Dim": "40", "Blind": "80", "Set": "28.0"},
		events: make(chan string, 16),
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOH) page() map[string]any {
	st := func(n string) string { return f.states[n] }
	sw := func(id, label, item, typ, state string, extra map[string]any) map[string]any {
		w := map[string]any{"widgetId": id, "type": "Switch", "label": label, "icon": "switch", "visibility": true,
			"item": map[string]any{"name": item, "type": typ, "state": state, "stateDescription": map[string]any{"readOnly": false}}}
		for k, v := range extra {
			w[k] = v
		}
		return w
	}
	slider := func(id, label, item, typ, state string, extra map[string]any) map[string]any {
		w := map[string]any{"widgetId": id, "type": "Slider", "label": label, "icon": "slider", "visibility": true,
			"item": map[string]any{"name": item, "type": typ, "state": state, "stateDescription": map[string]any{"readOnly": false}}}
		for k, v := range extra {
			w[k] = v
		}
		return w
	}
	locked := sw("w4", "Gesperrt [OFF]", "Locked", "Switch", st("Locked"), nil)
	locked["item"].(map[string]any)["stateDescription"] = map[string]any{"readOnly": true}
	return map[string]any{"id": "Fake", "title": "Fake Home", "widgets": []any{
		sw("w1", "Licht ["+st("Light")+"]", "Light", "Switch", st("Light"), map[string]any{"state": st("Light")}),
		sw("w2", "Ambient [0,0,0]", "Amb", "Color", st("Amb"), map[string]any{"state": map[bool]string{true: "ON", false: "OFF"}[st("Amb") != "0,0,0"]}),
		sw("w3", "Modus ["+st("Mode")+"]", "Mode", "Switch", st("Mode"), map[string]any{"mappings": []any{
			map[string]any{"command": "AUTO", "label": "Auto"}, map[string]any{"command": "AWAY", "label": "Abwesend"}}}),
		locked,
		slider("s1", "Dimmer ["+st("Dim")+" %]", "Dim", "Dimmer", st("Dim"), nil),
		slider("s2", "Rollo ["+st("Blind")+"]", "Blind", "Rollershutter", st("Blind"), nil),
		slider("s3", "Soll ["+st("Set")+"]", "Set", "Number", st("Set"), map[string]any{"minValue": 20.0, "maxValue": 50.0, "step": 0.5}),
		map[string]any{"widgetId": "f1", "type": "Frame", "label": "Stats", "visibility": true, "widgets": []any{
			map[string]any{"widgetId": "w5", "type": "Text", "label": "Leistung [" + st("Power") + " W]", "icon": "energy", "visibility": true,
				"item": map[string]any{"name": "Power", "type": "Number", "state": st("Power")}},
			map[string]any{"widgetId": "w6", "type": "Text", "label": "Versteckt [x]", "visibility": false,
				"item": map[string]any{"name": "Hidden", "type": "Number", "state": "1"}},
		}},
	}}
}

func (f *fakeOH) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/rest/sitemaps/Fake/Fake":
		f.mu.Lock()
		f.pageGets++
		page := f.page()
		f.mu.Unlock()
		json.NewEncoder(w).Encode(page)
	case r.URL.Path == "/rest/events":
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case block, ok := <-f.events:
				if !ok {
					return
				}
				fmt.Fprint(w, block)
				w.(http.Flusher).Flush()
			}
		}
	case strings.HasPrefix(r.URL.Path, "/rest/items/") && r.Method == http.MethodPost:
		item := strings.TrimPrefix(r.URL.Path, "/rest/items/")
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.ctype = r.Header.Get("Content-Type")
		f.commands = append(f.commands, item+"="+string(body))
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

// set changes an item like openHAB would and announces it on the event stream.
func (f *fakeOH) set(item, state string) {
	f.mu.Lock()
	f.states[item] = state
	f.mu.Unlock()
	f.events <- fmt.Sprintf("event: message\ndata: {\"topic\":\"openhab/items/%s/statechanged\",\"payload\":\"{}\",\"type\":\"ItemStateChangedEvent\"}\n\n", item)
}

func (f *fakeOH) gets() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pageGets
}

func startProvider(t *testing.T, f *fakeOH) *Provider {
	t.Helper()
	p := &Provider{client: &http.Client{Timeout: 5 * time.Second}, stream: sse.NewClient()}
	if err := p.Start(context.Background(), widget.Env{
		Connection: map[string]any{"url": f.srv.URL + "/"},
		Settings:   map[string]any{"sitemap": "Fake"},
	}); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- tests ----------------------------------------------------------------

func TestRowsFromRealFavsSitemap(t *testing.T) {
	raw, err := os.ReadFile("testdata/favs.json")
	if err != nil {
		t.Fatal(err)
	}
	var page pageResp
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	rows, controls, items := buildRows(page)

	if len(rows) != 6 { // 4 switches, frame, text
		t.Fatalf("rows = %d: %+v", len(rows), rows)
	}
	light := rows[0]
	if light.Kind != RowKindSwitch || light.Label != "Licht SZ" || light.On || light.Icon != "switch" {
		t.Errorf("light: %+v", light)
	}
	amb := rows[1] // a Color item operated by a Switch widget
	if amb.Kind != RowKindSwitch || amb.On || amb.Icon != "colorwheel" || amb.Label != "Amb. SZ" {
		t.Errorf("color switch: %+v", amb)
	}
	if rows[2].Label != "Amb. 1 WZ" || !rows[2].On {
		t.Errorf("AmbientLiving1 is ON in the fixture: %+v", rows[2])
	}
	if rows[4].Kind != RowKindFrame || rows[4].Label != "Stats" {
		t.Errorf("frame: %+v", rows[4])
	}
	if pv := rows[5]; pv.Kind != RowKindText || pv.Label != "PV Erz." || pv.Value != "0.0 W" {
		t.Errorf("text: %+v", pv)
	}
	if len(items) != 5 || len(controls) != 4 {
		t.Errorf("items=%v controls=%d", items, len(controls))
	}
}

func TestSplitLabel(t *testing.T) {
	for in, want := range map[string][2]string{
		"Licht SZ [OFF]":      {"Licht SZ", "OFF"},
		"PV Erz. [0.0 W]":     {"PV Erz.", "0.0 W"},
		"Nur Text":            {"Nur Text", ""},
		"Temp [21,5 °C]":      {"Temp", "21,5 °C"},
		"[42 %]":              {"", "42 %"},
		"Mit [Klammer] mitte": {"Mit [Klammer] mitte", ""},
		"Zwei [a] und [b c]":  {"Zwei [a] und", "b c"},
		"":                    {"", ""},
	} {
		if l, v := splitLabel(in); l != want[0] || v != want[1] {
			t.Errorf("%q -> %q, %q; want %q, %q", in, l, v, want[0], want[1])
		}
	}
}

func TestIsOn(t *testing.T) {
	item := func(state string) *itemResp { return &itemResp{State: state} }
	for _, c := range []struct {
		widgetState, itemState string
		want                   bool
	}{
		{"ON", "", true}, {"OFF", "", false}, {"", "ON", true}, {"", "NULL", false}, {"", "UNDEF", false},
		{"", "50", true}, {"", "0", false}, {"", "30,60,100", true}, {"", "0,0,0", false}, {"", "garbage", false},
	} {
		if got := isOn(widgetResp{State: c.widgetState}, item(c.itemState)); got != c.want {
			t.Errorf("widget=%q item=%q: got %v", c.widgetState, c.itemState, got)
		}
	}
}

func TestHiddenWidgetsAreSkipped(t *testing.T) {
	f := newFakeOH(t)
	d, err := startProvider(t, f).load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range d.Rows {
		if r.Label == "Versteckt" {
			t.Error("widget with visibility=false must not be shown")
		}
	}
	if d.Title != "Fake Home" {
		t.Errorf("title = %q", d.Title)
	}
}

func TestCommands(t *testing.T) {
	f := newFakeOH(t)
	p := startProvider(t, f)
	ctx := context.Background()
	if _, err := p.load(ctx); err != nil {
		t.Fatal(err)
	}
	do := func(widget, cmd string) error {
		_, err := p.Action(ctx, "command", json.RawMessage(fmt.Sprintf(`{"widget":%q,"command":%q}`, widget, cmd)))
		return err
	}

	// TOGGLE on an OFF switch sends ON, as text/plain to the item.
	if err := do("w1", "TOGGLE"); err != nil {
		t.Fatal(err)
	}
	if f.commands[0] != "Light=ON" || f.ctype != "text/plain" {
		t.Errorf("sent %v (%s)", f.commands, f.ctype)
	}
	// Once the page shows it as ON, TOGGLE sends OFF.
	f.states["Light"] = "ON"
	p.load(ctx)
	if err := do("w1", "TOGGLE"); err != nil || f.commands[1] != "Light=OFF" {
		t.Errorf("toggle off: %v %v", err, f.commands)
	}
	// Explicit ON/OFF and mapping commands.
	if err := do("w2", "ON"); err != nil || f.commands[2] != "Amb=ON" {
		t.Errorf("color switch: %v %v", err, f.commands)
	}
	if err := do("w3", "AWAY"); err != nil || f.commands[3] != "Mode=AWAY" {
		t.Errorf("mapping: %v %v", err, f.commands)
	}

	n := len(f.commands)
	for name, c := range map[string][2]string{
		"locked item":            {"w4", "ON"},
		"unknown widget":         {"w99", "ON"},
		"text row":               {"w5", "ON"},
		"frame":                  {"f1", "ON"},
		"switch with odd value":  {"w1", "50"},
		"switch with REFRESH":    {"w1", "REFRESH"},
		"mapping not in sitemap": {"w3", "SELF_DESTRUCT"},
		"item name as widget":    {"Light", "ON"},
	} {
		if err := do(c[0], c[1]); err == nil {
			t.Errorf("%s: must be refused", name)
		}
	}
	if len(f.commands) != n {
		t.Errorf("refused commands must not reach openHAB: %v", f.commands[n:])
	}
	if _, err := p.Action(ctx, "delete-everything", nil); err == nil {
		t.Error("unknown action must fail")
	}
}

func TestStreamPushesChangesAndIgnoresUnrelatedItems(t *testing.T) {
	f := newFakeOH(t)
	p := startProvider(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var latest Data
	updates := make(chan struct{}, 32)
	done := make(chan error, 1)
	go func() {
		done <- p.Stream(ctx, func(v any) {
			mu.Lock()
			latest = v.(Data)
			mu.Unlock()
			updates <- struct{}{}
		})
	}()
	lightOn := func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, r := range latest.Rows {
			if r.ID == "w1" {
				return r.On
			}
		}
		return false
	}
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for !cond() {
			select {
			case <-updates:
			case <-deadline:
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}

	waitFor("initial data", func() bool { mu.Lock(); defer mu.Unlock(); return len(latest.Rows) > 0 })
	if lightOn() {
		t.Fatal("light starts off")
	}

	f.set("Light", "ON")
	waitFor("light on", lightOn)

	// An item that is not on the page must not cause a re-read.
	time.Sleep(450 * time.Millisecond) // let pending refreshes settle
	before := f.gets()
	f.set("SomethingElse", "1")
	f.set("AnotherOne", "2")
	time.Sleep(700 * time.Millisecond)
	if after := f.gets(); after != before {
		t.Errorf("unrelated events caused %d page reads", after-before)
	}

	// A burst of related events is collapsed into few reads.
	before = f.gets()
	for i := 0; i < 10; i++ {
		f.set("Power", fmt.Sprint(i))
	}
	time.Sleep(900 * time.Millisecond)
	if n := f.gets() - before; n < 1 || n > 3 {
		t.Errorf("burst of 10 events caused %d reads, want 1-3", n)
	}

	// When the event stream ends, Stream returns so the core restarts it.
	close(f.events)
	select {
	case err := <-done:
		if err == nil {
			t.Error("Stream must return an error when the stream ends")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stream did not return after the event stream closed")
	}
}

func TestEventsURLSubscribesOnlyToShownItems(t *testing.T) {
	p := &Provider{base: "http://oh"}
	u := p.eventsURL([]string{"A", "B"})
	if !strings.Contains(u, "openhab%2Fitems%2FA%2Fstatechanged%2Copenhab%2Fitems%2FB%2Fstatechanged") {
		t.Errorf("url = %s", u)
	}
	many := make([]string, maxTopicItems+1)
	for i := range many {
		many[i] = fmt.Sprint("I", i)
	}
	if u := p.eventsURL(many); !strings.Contains(u, "items%2F%2A%2Fstatechanged") {
		t.Errorf("large pages must use the wildcard topic: %s", u)
	}
}

func TestStartValidation(t *testing.T) {
	for name, env := range map[string]widget.Env{
		"no connection": {Settings: map[string]any{"sitemap": "x"}},
		"bad url":       {Connection: map[string]any{"url": "nope"}, Settings: map[string]any{"sitemap": "x"}},
		"no sitemap":    {Connection: map[string]any{"url": "http://oh:8080"}, Settings: map[string]any{}},
	} {
		if err := (&Provider{}).Start(context.Background(), env); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestSlidersFromRealSitemap(t *testing.T) {
	raw, err := os.ReadFile("testdata/sliders.json")
	if err != nil {
		t.Fatal(err)
	}
	var page pageResp
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	rows, controls, _ := buildRows(page)
	if len(rows) != 4 {
		t.Fatalf("rows: %+v", rows)
	}
	level := func(r Row) float64 {
		if r.Level == nil {
			t.Fatalf("%s has no level", r.Label)
		}
		return *r.Level
	}
	dim, blind, color, num := rows[0], rows[1], rows[2], rows[3]
	if dim.Kind != RowKindSlider || level(dim) != 14 || dim.Min != 0 || dim.Max != 100 || dim.Step != 1 || dim.Value != "14 %" || dim.Shutter {
		t.Errorf("dimmer: %+v", dim)
	}
	if blind.Kind != RowKindSlider || level(blind) != 80 || !blind.Shutter || blind.Icon != "blinds" {
		t.Errorf("rollershutter: %+v", blind)
	}
	if color.Kind != RowKindSlider || level(color) != 0 { // brightness from the widget state, not "0,0,0"
		t.Errorf("color brightness: %+v", color)
	}
	if num.Kind != RowKindSlider || level(num) != 28 || num.Min != 20 || num.Max != 50 || num.Step != 1 {
		t.Errorf("number: %+v", num)
	}
	if len(controls) != 4 || controls["sl1"].itemType != "Rollershutter" {
		t.Errorf("controls: %+v", controls)
	}
}

func TestSliderCommands(t *testing.T) {
	f := newFakeOH(t)
	p := startProvider(t, f)
	ctx := context.Background()
	if _, err := p.load(ctx); err != nil {
		t.Fatal(err)
	}
	do := func(widget, cmd string) error {
		_, err := p.Action(ctx, "command", json.RawMessage(fmt.Sprintf(`{"widget":%q,"command":%q}`, widget, cmd)))
		return err
	}

	ok := []struct{ widget, cmd, sent string }{
		{"s1", "55", "Dim=55"},
		{"s1", "55.4", "Dim=55"}, // rounded to the step
		{"s1", "ON", "Dim=ON"},
		{"s1", "OFF", "Dim=OFF"},
		{"s1", "0", "Dim=0"},
		{"s1", "100", "Dim=100"},
		{"s2", "UP", "Blind=UP"},
		{"s2", "DOWN", "Blind=DOWN"},
		{"s2", "STOP", "Blind=STOP"},
		{"s2", "30", "Blind=30"},
		{"s3", "33.3", "Set=33.5"}, // step 0.5, min 20
		{"s3", "50", "Set=50"},
		{"s3", "20", "Set=20"},
	}
	for _, c := range ok {
		if err := do(c.widget, c.cmd); err != nil {
			t.Errorf("%s %s: %v", c.widget, c.cmd, err)
			continue
		}
		if got := f.commands[len(f.commands)-1]; got != c.sent {
			t.Errorf("%s %s sent %q, want %q", c.widget, c.cmd, got, c.sent)
		}
	}

	n := len(f.commands)
	for _, c := range []struct{ widget, cmd string }{
		{"s1", "101"}, {"s1", "-1"}, {"s1", "UP"}, {"s1", "STOP"}, {"s1", "INCREASE"}, {"s1", "abc"},
		{"s1", "NaN"}, {"s1", "Inf"}, {"s1", "1e999"}, {"s1", ""},
		{"s2", "ON"}, {"s2", "OFF"}, {"s2", "150"},
		{"s3", "19.9"}, {"s3", "50.1"}, {"s3", "ON"}, {"s3", "UP"},
	} {
		if err := do(c.widget, c.cmd); err == nil {
			t.Errorf("%s %q must be refused", c.widget, c.cmd)
		}
	}
	if len(f.commands) != n {
		t.Errorf("refused commands reached openHAB: %v", f.commands[n:])
	}
}

func TestSliderCommandRounding(t *testing.T) {
	c := control{kind: RowKindSlider, itemType: "Number", min: 0, max: 1, step: 0.1}
	for in, want := range map[string]string{"0.3": "0.3", "0.30000000000000004": "0.3", "0.74": "0.7", "0.76": "0.8", "1": "1", "0": "0"} {
		got, err := sliderCommand(c, in)
		if err != nil || got != want {
			t.Errorf("%s -> %q (%v), want %q", in, got, err, want)
		}
	}
}

func TestLevelOf(t *testing.T) {
	it := func(s string) *itemResp { return &itemResp{State: s} }
	f := func(v float64) *float64 { return &v }
	for _, c := range []struct {
		widget, item string
		want         *float64
	}{
		{"", "40", f(40)}, {"", "0,0,0", f(0)}, {"", "30,60,75", f(75)}, {"55", "1,2,3", f(55)},
		{"", "ON", f(100)}, {"", "OFF", f(0)}, {"", "NULL", nil}, {"", "UNDEF", nil}, {"", "", nil}, {"", "garbage", nil},
		{"NULL", "20", f(20)},
	} {
		got := levelOf(widgetResp{State: c.widget}, it(c.item))
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("widget=%q item=%q: got %v want %v", c.widget, c.item, got, c.want)
		}
	}
}
