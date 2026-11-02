// Package mqtt is the MQTT widget: it subscribes to one topic of an MQTT broker
// and shows the numeric value as a stat, gauge or chart. It exists because some
// sources (e.g. Grafana's MQTT datasource) are live-only and cannot be queried.
//
// The broker is configured as a connection:
//
//	connections:
//	  mqtt:
//	    url: mqtt.local:1883          # host[:port], optionally with tcp:// or ssl:// (mqtts://)
//	    username: lumina              # optional
//	    password: ${MQTT_PASSWORD}    # optional
//	    clientId: lumina              # optional; default lumina-<random>
//	    tls: true                     # optional; also implied by ssl://, mqtts://
//	    caFile: /certs/ca.pem         # optional; trust this CA instead of the system roots
//	    insecureSkipVerify: false     # optional; accept any server certificate
package mqtt

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/pmcgn/lumina/internal/history"
	"github.com/pmcgn/lumina/internal/widget"
)

const Type = "mqtt.value"

const (
	defaultRange  = 6 * time.Hour
	defaultPoints = 300
	connectWait   = 15 * time.Second
	keepAlive     = 30 * time.Second
)

func init() {
	widget.Register(Type, func() widget.Provider { return &Provider{now: time.Now} })
}

type Provider struct {
	now func() time.Time

	id       string
	s        Settings
	rng      time.Duration
	newOpts  func() *paho.ClientOptions // fresh options for every connection attempt
	series   string                     // name in the history store
	hist     *history.Store
	mu       sync.Mutex
	last     *history.Point // fallback when there is no history store
	warnings []string
}

var (
	rangeRe = regexp.MustCompile(`^([1-9][0-9]*)([smhdw])$`)
	colorRe = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]+|(rgb|hsl)a?\([0-9.,%\s/]+\))$`)
)

func parseRange(s string) (time.Duration, bool) {
	m := rangeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(m[1])
	unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}[m[2]]
	return time.Duration(n) * unit, true
}

// parseSettings decodes and validates the widget settings.
func parseSettings(m map[string]any) (Settings, error) {
	var s Settings
	if err := widget.DecodeSettings(m, &s); err != nil {
		return s, err
	}
	if s.Topic == "" {
		return s, fmt.Errorf("setting %q is required", "topic")
	}
	if strings.ContainsAny(s.Topic, "+#") {
		return s, fmt.Errorf("setting %q: wildcards are not supported, give the topic of one value", "topic")
	}
	switch s.Display {
	case "", DisplayChart, DisplayStat, DisplayGauge:
	default:
		return s, fmt.Errorf("setting %q: %q is invalid (stat, gauge, chart)", "display", s.Display)
	}
	if s.Range != "" {
		if _, ok := parseRange(s.Range); !ok {
			return s, fmt.Errorf("setting %q: %q is invalid (examples: 30m, 6h, 1d)", "range", s.Range)
		}
	}
	if s.Decimals != nil && (*s.Decimals < 0 || *s.Decimals > 10) {
		return s, fmt.Errorf("setting %q: must be 0..10", "decimals")
	}
	if s.Min != nil && s.Max != nil && *s.Min >= *s.Max {
		return s, errors.New("setting \"min\" must be smaller than \"max\"")
	}
	for i, t := range s.Thresholds {
		if t.Color != "" && !colorRe.MatchString(t.Color) {
			return s, fmt.Errorf("setting %q: step %d has an invalid color %q", "thresholds", i+1, t.Color)
		}
	}
	return s, nil
}

func (p *Provider) ValidateSettings(m map[string]any, _ map[string]map[string]any) error {
	_, err := parseSettings(m)
	return err
}

// brokerAddress turns the connection's url into a paho broker address
// (scheme://host:port). TLS is used for the schemes ssl, tls, mqtts and tcps,
// or when tlsOn is set; plain TCP is the default (port 1883, with TLS 8883).
func brokerAddress(raw string, tlsOn bool) (addr string, useTLS bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, errors.New("connection.url: missing (e.g. mqtt.local:1883)")
	}
	if !strings.Contains(raw, "://") {
		raw = "tcp://" + raw
	}
	u, perr := url.Parse(raw)
	if perr != nil || u.Hostname() == "" {
		return "", false, fmt.Errorf("connection.url: %q is not a valid broker address (e.g. mqtt.local:1883)", raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "tcp", "mqtt":
	case "ssl", "tls", "mqtts", "tcps":
		tlsOn = true
	default:
		return "", false, fmt.Errorf("connection.url: scheme %q is not supported (use tcp, mqtt, ssl or mqtts)", u.Scheme)
	}
	if u.Path != "" && u.Path != "/" {
		return "", false, errors.New("connection.url: must not contain a path")
	}
	port := u.Port()
	scheme := "tcp"
	if tlsOn {
		scheme = "ssl"
		if port == "" {
			port = "8883"
		}
	} else if port == "" {
		port = "1883"
	}
	return scheme + "://" + net.JoinHostPort(u.Hostname(), port), tlsOn, nil
}

// clientID is the user's client name with the widget id appended (a broker
// disconnects a client when another one connects with the same id, and every
// mqtt widget is its own client), or lumina-<random> if none is configured.
func clientID(configured, widgetID string) (string, error) {
	if configured != "" {
		if widgetID != "" {
			return configured + "-" + widgetID, nil
		}
		return configured, nil
	}
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("client id: %w", err)
	}
	return "lumina-" + hex.EncodeToString(b), nil
}

// tlsConfig verifies the broker's certificate against the CA from caFile, or,
// if none is configured, against the system trust store (in the Docker image
// the ca-certificates bundle of Alpine).
func tlsConfig(c map[string]any) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: widget.Bool(c, "insecureSkipVerify", false)}
	file := widget.String(c, "caFile", "")
	if file == "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			return nil, fmt.Errorf("connection: the system CA certificates cannot be loaded (%v): set caFile", err)
		}
		cfg.RootCAs = pool
		return cfg, nil
	}
	pem, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("connection.caFile: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("connection.caFile: %s contains no PEM certificate", file)
	}
	cfg.RootCAs = pool
	return cfg, nil
}

func (p *Provider) Start(_ context.Context, env widget.Env) error {
	c := env.Connection
	if c == nil {
		return errors.New("connection: this widget needs a connection with the broker url")
	}
	s, err := parseSettings(env.Settings)
	if err != nil {
		return err
	}
	addr, useTLS, err := brokerAddress(widget.String(c, "url", ""), widget.Bool(c, "tls", false))
	if err != nil {
		return err
	}
	id, err := clientID(widget.String(c, "clientId", ""), env.ID)
	if err != nil {
		return err
	}
	user, pass := widget.String(c, "username", ""), widget.String(c, "password", "")
	var tlsCfg *tls.Config
	if useTLS {
		if tlsCfg, err = tlsConfig(c); err != nil {
			return err
		}
	}
	p.newOpts = func() *paho.ClientOptions {
		opts := paho.NewClientOptions().
			AddBroker(addr).
			SetClientID(id).
			SetCleanSession(true).
			SetKeepAlive(keepAlive).
			SetConnectTimeout(connectWait).
			SetAutoReconnect(false) // a lost connection ends Stream, the core reconnects with backoff
		if user != "" {
			opts.SetUsername(user).SetPassword(pass)
		}
		if tlsCfg != nil {
			opts.SetTLSConfig(tlsCfg.Clone())
		}
		return opts
	}
	p.id, p.s, p.hist = env.ID, s, env.History
	p.series = env.ID + "/" + s.Topic
	p.rng = defaultRange
	if s.Range != "" {
		p.rng, _ = parseRange(s.Range)
	}
	return nil
}

// extractValue reads the number out of a message: the payload itself, or a
// field of a JSON payload (path "a.b"; default "value" for JSON objects).
func extractValue(payload []byte, path string) (float64, error) {
	text := strings.TrimSpace(string(payload))
	if text == "" {
		return 0, errors.New("empty payload")
	}
	if text[0] != '{' && text[0] != '[' && path == "" {
		return parseNumber(text)
	}
	v, err := parseJSON(text)
	if err != nil {
		if path == "" {
			return parseNumber(text)
		}
		return 0, errors.New("payload is not JSON")
	}
	if path == "" {
		path = "value"
	}
	for _, key := range strings.Split(path, ".") {
		switch node := v.(type) {
		case map[string]any:
			next, ok := node[key]
			if !ok {
				return 0, fmt.Errorf("no field %q in the payload", path)
			}
			v = next
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(node) {
				return 0, fmt.Errorf("no element %q in the payload", path)
			}
			v = node[i]
		default:
			return 0, fmt.Errorf("no field %q in the payload", path)
		}
	}
	switch n := v.(type) {
	case float64:
		return n, nil
	case string:
		return parseNumber(n)
	}
	return 0, fmt.Errorf("field %q is not a number", path)
}

func parseNumber(s string) (float64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f != f || f > 1e300 || f < -1e300 {
		return 0, errors.New("payload is not a number")
	}
	return f, nil
}

// data builds the model from what was received so far.
func (p *Provider) data() Data {
	now := p.now()
	from := now.Add(-p.rng)
	var pts []Point
	if p.hist != nil {
		for _, h := range p.hist.Query(p.series, from, now, defaultPoints) {
			pts = append(pts, Point{T: h.T, V: h.V})
		}
	}
	if len(pts) == 0 && p.last != nil { // no history store, or its series cap is reached
		pts = []Point{{T: p.last.T, V: p.last.V}}
	}
	d := Data{
		Title:    firstNonEmpty(p.s.Title, p.s.Topic),
		Display:  p.s.Display,
		Unit:     p.s.Unit,
		Decimals: p.s.Decimals,
		Min:      p.s.Min,
		Max:      p.s.Max,
		Series:   []Series{},
		From:     from.UnixMilli(),
		To:       now.UnixMilli(),
		Warnings: p.warnings,
	}
	if d.Display == "" {
		d.Display = DisplayStat
	}
	d.Thresholds = append(d.Thresholds, p.s.Thresholds...)
	if len(pts) > 0 {
		d.Series = []Series{{Points: pts}}
	}
	return d
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Stream implements widget.Streamer. It returns when the connection is lost or
// cannot be made; the core restarts it with backoff.
func (p *Provider) Stream(ctx context.Context, emit func(any)) error {
	poke := make(chan struct{}, 1)
	lost := make(chan error, 1)
	var payload []byte
	var pmu sync.Mutex

	opts := p.newOpts()
	opts.SetConnectionLostHandler(func(_ paho.Client, err error) {
		select {
		case lost <- fmt.Errorf("connection lost: %w", err):
		default:
		}
	})
	client := paho.NewClient(opts)
	tok := client.Connect()
	if !waitToken(ctx, tok, connectWait+time.Second) {
		client.Disconnect(0)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("connect: timeout")
	}
	if err := tok.Error(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(250)

	// Only the newest message matters: keep the last one and poke the loop.
	sub := client.Subscribe(p.s.Topic, 0, func(_ paho.Client, m paho.Message) {
		pmu.Lock()
		payload = append(payload[:0], m.Payload()...)
		pmu.Unlock()
		select {
		case poke <- struct{}{}:
		default:
		}
	})
	if !waitToken(ctx, sub, connectWait) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("subscribe: timeout")
	}
	if err := sub.Error(); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	emit(p.data()) // connected: show the empty state until the first message arrives

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-lost:
			return err
		case <-poke:
			pmu.Lock()
			msg := append([]byte(nil), payload...)
			pmu.Unlock()
			p.mu.Lock()
			p.record(msg)
			d := p.data()
			p.mu.Unlock()
			emit(d)
		}
	}
}

// record stores a message's value, or remembers why it could not be read.
func (p *Provider) record(msg []byte) {
	v, err := extractValue(msg, p.s.ValuePath)
	if err != nil {
		p.warnings = []string{err.Error()}
		return
	}
	p.warnings = nil
	now := p.now()
	if p.hist != nil {
		p.hist.Add(p.series, now, v)
	}
	p.last = &history.Point{T: now.UnixMilli(), V: v}
}

func parseJSON(text string) (any, error) {
	var v any
	err := json.Unmarshal([]byte(text), &v)
	return v, err
}

func waitToken(ctx context.Context, t paho.Token, d time.Duration) bool {
	select {
	case <-t.Done():
		return true
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return false
	}
}
