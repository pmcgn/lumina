package mqtt

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/history"
	"github.com/pmcgn/lumina/internal/widget"
)

// fakeBroker is a minimal MQTT 3.1.1 broker: it accepts one client at a time,
// records the CONNECT credentials and publishes to every subscription.
type fakeBroker struct {
	ln net.Listener

	mu       sync.Mutex
	clientID string
	user     string
	pass     string
	hasUser  bool
	topic    string
	conn     net.Conn
	rejectRC byte // CONNACK return code
	subscr   chan struct{}
}

func newBroker(t *testing.T) *fakeBroker {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &fakeBroker{ln: ln, subscr: make(chan struct{}, 4)}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go b.serve(c)
		}
	}()
	return b
}

func (b *fakeBroker) addr() string { return b.ln.Addr().String() }

func readPacket(r io.Reader) (typ byte, body []byte, err error) {
	var h [1]byte
	if _, err = io.ReadFull(r, h[:]); err != nil {
		return
	}
	n, shift := 0, 0
	for {
		var x [1]byte
		if _, err = io.ReadFull(r, x[:]); err != nil {
			return
		}
		n |= int(x[0]&0x7f) << shift
		if x[0]&0x80 == 0 {
			break
		}
		shift += 7
	}
	body = make([]byte, n)
	_, err = io.ReadFull(r, body)
	return h[0], body, err
}

func str(b []byte) (string, []byte) {
	n := int(binary.BigEndian.Uint16(b))
	return string(b[2 : 2+n]), b[2+n:]
}

func (b *fakeBroker) serve(c net.Conn) {
	defer c.Close()
	for {
		typ, body, err := readPacket(c)
		if err != nil {
			return
		}
		switch typ >> 4 {
		case 1: // CONNECT
			_, rest := str(body) // protocol name
			flags := rest[1]
			rest = rest[4:] // level, flags, keepalive
			b.mu.Lock()
			b.clientID, rest = str(rest)
			if flags&0x80 != 0 {
				b.hasUser = true
				b.user, rest = str(rest)
			}
			if flags&0x40 != 0 {
				b.pass, _ = str(rest)
			}
			rc := b.rejectRC
			b.mu.Unlock()
			c.Write([]byte{0x20, 2, 0, rc})
			if rc != 0 {
				return
			}
		case 8: // SUBSCRIBE
			id := body[:2]
			topic, _ := str(body[2:])
			b.mu.Lock()
			b.topic, b.conn = topic, c
			b.mu.Unlock()
			c.Write([]byte{0x90, 3, id[0], id[1], 0})
			b.subscr <- struct{}{}
		case 12: // PINGREQ
			c.Write([]byte{0xd0, 0})
		case 14: // DISCONNECT
			return
		}
	}
}

func (b *fakeBroker) publish(payload string) {
	b.mu.Lock()
	c, topic := b.conn, b.topic
	b.mu.Unlock()
	body := append([]byte{byte(len(topic) >> 8), byte(len(topic))}, topic...)
	body = append(body, payload...)
	c.Write(append([]byte{0x30, byte(len(body))}, body...)) // short messages only
}

func (b *fakeBroker) drop() {
	b.mu.Lock()
	c := b.conn
	b.mu.Unlock()
	c.Close()
}

var now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

func startProvider(t *testing.T, conn, settings map[string]any, hist *history.Store) *Provider {
	t.Helper()
	p := &Provider{now: func() time.Time { return now }}
	err := p.Start(context.Background(), widget.Env{ID: "battery", Settings: settings, Connection: conn, History: hist})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type stream struct {
	out  chan Data
	done chan error
	stop context.CancelFunc
}

func runStream(t *testing.T, p *Provider) *stream {
	ctx, cancel := context.WithCancel(context.Background())
	s := &stream{out: make(chan Data, 16), done: make(chan error, 1), stop: cancel}
	go func() { s.done <- p.Stream(ctx, func(v any) { s.out <- v.(Data) }) }()
	t.Cleanup(cancel)
	return s
}

func (s *stream) next(t *testing.T) Data {
	t.Helper()
	select {
	case d := <-s.out:
		return d
	case err := <-s.done:
		t.Fatalf("stream ended: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("no data")
	}
	return Data{}
}

func TestStreamDeliversValuesWithCredentialsAndClientName(t *testing.T) {
	b := newBroker(t)
	p := startProvider(t, map[string]any{"url": b.addr(), "username": "lumina", "password": "s3cret", "clientId": "wall"},
		map[string]any{"topic": "victron/dc/soc", "display": "gauge", "unit": "percent"}, history.New(0, 0))
	s := runStream(t, p)

	if d := s.next(t); len(d.Series) != 0 || d.Display != DisplayGauge {
		t.Fatalf("connecting must first publish the empty state: %+v", d)
	}
	<-b.subscr
	b.publish(`{"value": 87.5}`)
	d := s.next(t)
	if len(d.Series) != 1 || d.Series[0].Points[len(d.Series[0].Points)-1].V != 87.5 || d.Title != "victron/dc/soc" || d.Unit != "percent" {
		t.Errorf("unexpected data: %+v", d)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.topic != "victron/dc/soc" || !b.hasUser || b.user != "lumina" || b.pass != "s3cret" {
		t.Errorf("credentials or topic not sent: topic=%q user=%q pass=%q", b.topic, b.user, b.pass)
	}
	if b.clientID != "wall-battery" {
		t.Errorf("the client name must be the configured one plus the widget id, got %q", b.clientID)
	}
}

func TestAnonymousConnectionAndRandomClientName(t *testing.T) {
	b := newBroker(t)
	p := startProvider(t, map[string]any{"url": "tcp://" + b.addr()}, map[string]any{"topic": "t"}, nil)
	s := runStream(t, p)
	s.next(t)
	<-b.subscr
	b.publish("42")
	if d := s.next(t); len(d.Series) != 1 || d.Series[0].Points[0].V != 42 {
		t.Errorf("a plain number must work without a history store: %+v", d)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.hasUser {
		t.Error("no credentials configured, none may be sent")
	}
	if !strings.HasPrefix(b.clientID, "lumina-") || len(b.clientID) <= len("lumina-") {
		t.Errorf("default client name: %q", b.clientID)
	}
}

func TestClientNamesAreRandomPerWidget(t *testing.T) {
	a, _ := clientID("", "w1")
	b, _ := clientID("", "w1")
	if a == b {
		t.Errorf("default names must be random: %q == %q", a, b)
	}
	if got, _ := clientID("home", ""); got != "home" {
		t.Errorf("got %q", got)
	}
}

func TestBadPayloadWarnsAndRecovers(t *testing.T) {
	b := newBroker(t)
	p := startProvider(t, map[string]any{"url": b.addr()}, map[string]any{"topic": "t"}, history.New(0, 0))
	s := runStream(t, p)
	s.next(t)
	<-b.subscr
	b.publish("offline")
	if d := s.next(t); len(d.Warnings) != 1 || !strings.Contains(d.Warnings[0], "not a number") {
		t.Errorf("expected a warning: %+v", d)
	}
	b.publish("5")
	if d := s.next(t); len(d.Warnings) != 0 || len(d.Series) != 1 {
		t.Errorf("a valid value clears the warning: %+v", d)
	}
}

func TestRejectedAndLostConnectionsEndTheStream(t *testing.T) {
	b := newBroker(t)
	b.rejectRC = 5 // not authorized
	p := startProvider(t, map[string]any{"url": b.addr(), "username": "u", "password": "wrong"}, map[string]any{"topic": "t"}, nil)
	if err := p.Stream(context.Background(), func(any) {}); err == nil || !strings.Contains(err.Error(), "connect") {
		t.Errorf("a refused login must be an error: %v", err)
	}

	b.mu.Lock()
	b.rejectRC = 0
	b.mu.Unlock()
	s := runStream(t, p)
	s.next(t)
	<-b.subscr
	b.drop()
	select {
	case err := <-s.done:
		if err == nil || !strings.Contains(err.Error(), "connection lost") {
			t.Errorf("a lost connection must end the stream so the core reconnects: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not end")
	}
}

func TestExtractValue(t *testing.T) {
	cases := []struct {
		payload, path string
		want          float64
		err           string
	}{
		{"87.5", "", 87.5, ""},
		{" 3 \n", "", 3, ""},
		{`{"value": 12}`, "", 12, ""},
		{`{"value": "12.5"}`, "", 12.5, ""},
		{`{"data": {"soc": 7}}`, "data.soc", 7, ""},
		{`{"a": [1, 2, 3]}`, "a.1", 2, ""},
		{`[4, 5]`, "1", 5, ""},
		{"", "", 0, "empty"},
		{"on", "", 0, "not a number"},
		{`{"x": 1}`, "", 0, `no field "value"`},
		{`{"value": true}`, "", 0, "not a number"},
		{"5", "value", 0, "no field"},
		{"abc", "value", 0, "not JSON"},
		{"NaN", "", 0, "not a number"},
	}
	for _, c := range cases {
		got, err := extractValue([]byte(c.payload), c.path)
		switch {
		case c.err == "" && (err != nil || got != c.want):
			t.Errorf("%q (%q) = %v, %v; want %v", c.payload, c.path, got, err, c.want)
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%q (%q): want error %q, got %v", c.payload, c.path, c.err, err)
		}
	}
}

func TestBrokerAddress(t *testing.T) {
	cases := []struct {
		in      string
		tls     bool
		want    string
		wantTLS bool
		err     string
	}{
		{"mqtt.local", false, "tcp://mqtt.local:1883", false, ""},
		{"mqtt.local:1884", false, "tcp://mqtt.local:1884", false, ""},
		{"tcp://mqtt.local:1883", false, "tcp://mqtt.local:1883", false, ""},
		{"mqtt://10.0.0.5", false, "tcp://10.0.0.5:1883", false, ""},
		{"mqtt.local", true, "ssl://mqtt.local:8883", true, ""},
		{"mqtts://mqtt.local", false, "ssl://mqtt.local:8883", true, ""},
		{"ssl://mqtt.local:8884", false, "ssl://mqtt.local:8884", true, ""},
		{"tcp://mqtt.local:8883", true, "ssl://mqtt.local:8883", true, ""},
		{"[::1]:1883", false, "tcp://[::1]:1883", false, ""},
		{"", false, "", false, "missing"},
		{"http://mqtt.local", false, "", false, "not supported"},
		{"mqtt.local/x", false, "", false, "path"},
	}
	for _, c := range cases {
		got, useTLS, err := brokerAddress(c.in, c.tls)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%q: want error %q, got %v", c.in, c.err, err)
			}
			continue
		}
		if err != nil || got != c.want || useTLS != c.wantTLS {
			t.Errorf("%q (tls=%v) = %q tls=%v err=%v; want %q tls=%v", c.in, c.tls, got, useTLS, err, c.want, c.wantTLS)
		}
	}
}

func TestTLSDefaultsToTheSystemTrustStore(t *testing.T) {
	cfg, err := tlsConfig(map[string]any{})
	if err != nil {
		t.Skipf("no system CA certificates on this machine: %v", err)
	}
	if cfg.RootCAs == nil || cfg.InsecureSkipVerify {
		t.Errorf("without caFile the system roots must be used and verification stay on: %+v", cfg)
	}
}

func TestStartValidation(t *testing.T) {
	p := &Provider{now: time.Now}
	if err := p.Start(context.Background(), widget.Env{Settings: map[string]any{"topic": "t"}}); err == nil || !strings.Contains(err.Error(), "connection") {
		t.Errorf("a missing connection must be reported: %v", err)
	}
	err := p.Start(context.Background(), widget.Env{Settings: map[string]any{"topic": "t"}, Connection: map[string]any{"url": "x", "tls": true, "caFile": "/nonexistent/ca.pem"}})
	if err == nil || !strings.Contains(err.Error(), "caFile") {
		t.Errorf("an unreadable CA file must be reported: %v", err)
	}
	if err := p.Start(context.Background(), widget.Env{Settings: map[string]any{"topic": "t"}, Connection: map[string]any{"url": "x", "tls": true, "insecureSkipVerify": true}}); err != nil {
		t.Errorf("tls without a CA file uses the system roots: %v", err)
	}
}
