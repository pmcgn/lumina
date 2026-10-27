// openHAB stand-in: a sitemap shaped like a real one, a working command
// endpoint with simulated devices, and a live event stream.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type openhabSim struct {
	mu       sync.Mutex
	states   map[string]string
	clients  map[chan string]struct{}
	blindGen int // cancels a running blind movement when a new command arrives
}

func newOpenHAB() *openhabSim {
	s := &openhabSim{
		states: map[string]string{
			"LivingLight": "ON", "KitchenLight": "OFF", "HallLight": "OFF", "Ambient": "0,0,0",
			"HeatingMode": "AUTO", "Locked": "OFF", "PV": "1432.0", "RoomTemp": "21.4",
			"Dimmer": "40", "Blind": "80", "Setpoint": "21.0",
		},
		clients: map[chan string]struct{}{},
	}
	go s.simulateSensors()
	return s
}

func (s *openhabSim) register(mux *http.ServeMux) {
	mux.HandleFunc("/rest/sitemaps/Demo/Demo", s.page)
	mux.HandleFunc("/rest/events", s.events)
	mux.HandleFunc("/rest/items/", s.command)
}

func (s *openhabSim) widget(id, typ, label, icon, item, itype string, extra map[string]any) map[string]any {
	w := map[string]any{"widgetId": id, "type": typ, "label": label, "icon": icon, "visibility": true}
	if item != "" {
		w["item"] = map[string]any{"name": item, "type": itype, "state": s.states[item], "stateDescription": map[string]any{"readOnly": false}}
	}
	for k, v := range extra {
		w[k] = v
	}
	return w
}

func (s *openhabSim) page(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.states
	sw := func(id, name, icon, item string) map[string]any {
		return s.widget(id, "Switch", fmt.Sprintf("%s [%s]", name, st[item]), icon, item, "Switch", map[string]any{"state": st[item]})
	}
	ambState := "OFF"
	if st["Ambient"] != "0,0,0" {
		ambState = "ON"
	}
	locked := s.widget("w6", "Switch", "Alarmanlage ["+st["Locked"]+"]", "switch", "Locked", "Switch", nil)
	locked["item"].(map[string]any)["stateDescription"] = map[string]any{"readOnly": true}

	json.NewEncoder(w).Encode(map[string]any{"id": "Demo", "title": "Demo-Zuhause", "widgets": []any{
		s.widget("f1", "Frame", "Licht", "frame", "", "", map[string]any{"widgets": []any{
			sw("w1", "Wohnzimmer", "light", "LivingLight"),
			sw("w2", "Küche", "light", "KitchenLight"),
			sw("w3", "Flur", "light", "HallLight"),
			s.widget("w4", "Switch", "Ambilight ["+st["Ambient"]+"]", "colorwheel", "Ambient", "Color", map[string]any{"state": ambState}),
			locked,
		}}),
		s.widget("f4", "Frame", "Helligkeit und Rollläden", "frame", "", "", map[string]any{"widgets": []any{
			s.widget("w9", "Slider", "Dimmer Nachttisch ["+st["Dimmer"]+" %]", "light", "Dimmer", "Dimmer", nil),
			s.widget("w10", "Slider", "Rollladen Fenster ["+st["Blind"]+"]", "blinds", "Blind", "Rollershutter", nil),
		}}),
		s.widget("f2", "Frame", "Heizung", "frame", "", "", map[string]any{"widgets": []any{
			s.widget("w11", "Slider", "Soll-Temperatur ["+st["Setpoint"]+" °C]", "temperature", "Setpoint", "Number", map[string]any{"minValue": 16.0, "maxValue": 26.0, "step": 0.5}),
			s.widget("w5", "Switch", "Modus ["+st["HeatingMode"]+"]", "temperature", "HeatingMode", "Switch", map[string]any{"mappings": []any{
				map[string]any{"command": "AUTO", "label": "Auto"},
				map[string]any{"command": "COMFORT", "label": "Komfort"},
				map[string]any{"command": "AWAY", "label": "Abwesend"},
			}}),
			s.widget("w7", "Text", "Raumtemperatur ["+st["RoomTemp"]+" °C]", "temperature", "RoomTemp", "Number", nil),
		}}),
		s.widget("f3", "Frame", "Stats", "frame", "", "", map[string]any{"widgets": []any{
			s.widget("w8", "Text", "PV Erz. ["+st["PV"]+" W]", "solarplant", "PV", "Number", nil),
		}}),
	}})
}

func (s *openhabSim) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	f := w.(http.Flusher)
	f.Flush()

	ch := make(chan string, 32)
	s.mu.Lock()
	s.clients[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, ch)
		s.mu.Unlock()
	}()

	alive := time.NewTicker(5 * time.Second)
	defer alive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprint(w, msg)
			f.Flush()
		case <-alive.C:
			fmt.Fprint(w, "event: alive\ndata: {\"type\":\"ALIVE\",\"interval\":5}\n\n")
			f.Flush()
		}
	}
}

// command handles POST /rest/items/{name} with a plain-text command body.
func (s *openhabSim) command(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	item := strings.TrimPrefix(r.URL.Path, "/rest/items/")
	body, _ := io.ReadAll(io.LimitReader(r.Body, 256))
	cmd := strings.TrimSpace(string(body))

	s.mu.Lock()
	_, known := s.states[item]
	s.mu.Unlock()
	if !known {
		http.NotFound(w, r)
		return
	}
	log.Printf("command: %s <- %s", item, cmd)
	switch {
	case item == "Ambient" && cmd == "ON":
		cmd = "30,60,100"
	case item == "Ambient" && cmd == "OFF":
		cmd = "0,0,0"
	case item == "Dimmer" && cmd == "ON":
		cmd = "100"
	case item == "Dimmer" && cmd == "OFF":
		cmd = "0"
	case item == "Blind":
		s.moveBlind(cmd)
		w.WriteHeader(http.StatusOK)
		return
	}
	// A real device takes a moment to answer.
	go func() {
		time.Sleep(350 * time.Millisecond)
		s.set(item, cmd)
	}()
	w.WriteHeader(http.StatusOK)
}

func (s *openhabSim) set(item, state string) {
	s.mu.Lock()
	old := s.states[item]
	s.states[item] = state
	if old == state {
		s.mu.Unlock()
		return
	}
	payload, _ := json.Marshal(map[string]string{"type": "String", "value": state, "oldType": "String", "oldValue": old})
	msg, _ := json.Marshal(map[string]string{"topic": "openhab/items/" + item + "/statechanged", "payload": string(payload), "type": "ItemStateChangedEvent"})
	block := "event: message\ndata: " + string(msg) + "\n\n"
	for ch := range s.clients {
		select {
		case ch <- block:
		default:
		}
	}
	s.mu.Unlock()
}

// simulateSensors makes the value tiles move, like a live installation.
func (s *openhabSim) simulateSensors() {
	for range time.Tick(3 * time.Second) {
		s.mu.Lock()
		var pv, temp float64
		fmt.Sscan(s.states["PV"], &pv)
		fmt.Sscan(s.states["RoomTemp"], &temp)
		s.mu.Unlock()
		pv = max(0, pv+float64(rand.Intn(240)-110))
		temp += float64(rand.Intn(5)-2) / 10
		s.set("PV", fmt.Sprintf("%.1f", pv))
		s.set("RoomTemp", fmt.Sprintf("%.1f", temp))
	}
}

// moveBlind simulates a rollershutter: UP/DOWN/percent move step by step and
// STOP halts it, so the live position updates are visible in the dashboard.
func (s *openhabSim) moveBlind(cmd string) {
	s.mu.Lock()
	s.blindGen++
	gen := s.blindGen
	var cur float64
	fmt.Sscan(s.states["Blind"], &cur)
	s.mu.Unlock()

	var target float64
	switch cmd {
	case "STOP":
		return
	case "UP":
		target = 0
	case "DOWN":
		target = 100
	default:
		if _, err := fmt.Sscan(cmd, &target); err != nil {
			return
		}
	}
	go func() {
		for cur != target {
			time.Sleep(400 * time.Millisecond)
			s.mu.Lock()
			cancelled := s.blindGen != gen
			s.mu.Unlock()
			if cancelled {
				return
			}
			switch {
			case cur < target:
				cur = min(cur+10, target)
			default:
				cur = max(cur-10, target)
			}
			s.set("Blind", fmt.Sprintf("%.0f", cur))
		}
	}()
}
