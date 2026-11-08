// Package tibber is the Tibber electricity price widget. It uses the Tibber Data
// API (OAuth2, scope data-api-prices-read) and shows the current price and the
// published 15-minute prices of today and tomorrow. The refresh token is
// obtained once with cmd/tibber-auth (see WIDGETS.md).
package tibber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pmcgn/lumina/internal/httpx"
	"github.com/pmcgn/lumina/internal/widget"
)

const Type = "tibber.prices"

const (
	defaultAPIURL = "https://data-api.tibber.com"
	stepSec       = 900
	maxPages      = 4 // a page holds up to 48 h, so today + tomorrow need two
)

func init() {
	widget.Register(Type, func() widget.Provider { return newProvider() })
}

type Provider struct {
	apiURL   string
	tokenURL string
	client   *http.Client
	now      func() time.Time

	label string
	home  string // setting: id or name
	loc   *time.Location
	ts    *tokenSource

	mu       sync.Mutex
	homeID   string
	homeName string
	cache    *Data
	cachedAt time.Time
}

func newProvider() *Provider {
	return &Provider{apiURL: defaultAPIURL, tokenURL: defaultTokenURL, client: httpx.NewClient(), now: time.Now}
}

func (p *Provider) ValidateSettings(m map[string]any, _ map[string]map[string]any) error {
	var s Settings
	return widget.DecodeSettings(m, &s)
}

func (p *Provider) Start(_ context.Context, env widget.Env) error {
	var s Settings
	if err := widget.DecodeSettings(env.Settings, &s); err != nil {
		return err
	}
	c := env.Connection
	if c == nil {
		return errors.New("connection: this widget needs a connection with clientId, clientSecret and refreshToken")
	}
	id := widget.String(c, "clientId", "")
	secret := widget.String(c, "clientSecret", "")
	refresh := widget.String(c, "refreshToken", "")
	switch {
	case id == "":
		return errors.New("connection.clientId: missing (the OAuth client from data-api.tibber.com/clients/manage)")
	case secret == "":
		return errors.New("connection.clientSecret: missing")
	case refresh == "":
		return errors.New("connection.refreshToken: missing (create one with: go run ./cmd/tibber-auth)")
	}
	file := widget.String(c, "tokenFile", "tibber-token.json")
	// url / tokenUrl point at a stand-in service (the demo); the defaults are Tibber itself.
	p.apiURL = strings.TrimRight(widget.String(c, "url", p.apiURL), "/")
	p.tokenURL = widget.String(c, "tokenUrl", p.tokenURL)
	p.ts = sharedSource(p.client, p.tokenURL, id, secret, refresh, file)
	p.home, p.label = s.Home, s.Label
	p.loc = env.Location
	if p.loc == nil {
		p.loc = time.Local
	}
	return nil
}

func (p *Provider) Interval() time.Duration { return 15 * time.Minute }

func (p *Provider) Fetch(ctx context.Context) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if !p.stale(now) {
		return *p.cache, nil
	}
	d, err := p.load(ctx, now)
	if err != nil {
		return nil, err
	}
	p.cache, p.cachedAt = &d, now
	return d, nil
}

// stale reports whether the cached prices must be fetched again. Published
// prices never change, so we only fetch when the day changed, or while
// tomorrow's prices (published around 13:00 CET) are still missing.
func (p *Provider) stale(now time.Time) bool {
	if p.cache == nil {
		return true
	}
	today := dayStart(now, p.loc)
	if dayStart(p.cachedAt, p.loc) != today {
		return true
	}
	if n := len(p.cache.Slots); n > 0 && p.cache.Slots[n-1].T+stepSec >= today.AddDate(0, 0, 2).Unix() {
		return false // tomorrow is complete
	}
	return now.UTC().Hour() >= 11 && now.Sub(p.cachedAt) >= time.Hour
}

func dayStart(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func (p *Provider) load(ctx context.Context, now time.Time) (Data, error) {
	if p.homeID == "" {
		if err := p.resolveHome(ctx); err != nil {
			return Data{}, err
		}
	}
	start := dayStart(now, p.loc)
	next := p.apiURL + "/v1/homes/" + url.PathEscape(p.homeID) + "/prices?since=" + url.QueryEscape(start.Format(time.RFC3339))
	d := Data{Label: p.label, Timezone: p.loc.String(), StepSec: stepSec}
	if d.Label == "" {
		d.Label = p.homeName
	}
	seen := map[int64]bool{}
	for page := 0; page < maxPages && next != ""; page++ {
		var r pricesResponse
		if err := p.get(ctx, next, &r); err != nil {
			return Data{}, err
		}
		d.Currency = r.Currency
		added := 0
		for _, it := range r.Items {
			t, err := time.Parse(time.RFC3339, it.Time)
			if err != nil || t.Before(start) || seen[t.Unix()] {
				continue
			}
			seen[t.Unix()] = true
			d.Slots = append(d.Slots, Slot{T: t.Unix(), Price: float64(it.Consumption.Price)})
			added++
		}
		if len(r.Items) == 0 || added == 0 || r.Next == "" {
			break
		}
		ref, err := url.Parse(r.Next)
		if err != nil {
			return Data{}, fmt.Errorf("tibber: bad next link: %w", err)
		}
		base, _ := url.Parse(next)
		next = base.ResolveReference(ref).String()
	}
	if len(d.Slots) == 0 {
		return Data{}, errors.New("tibber: no prices published for today yet")
	}
	sort.Slice(d.Slots, func(i, j int) bool { return d.Slots[i].T < d.Slots[j].T })
	return d, nil
}

func (p *Provider) resolveHome(ctx context.Context) error {
	var r struct {
		Homes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"homes"`
	}
	if err := p.get(ctx, p.apiURL+"/v1/homes", &r); err != nil {
		return err
	}
	for _, h := range r.Homes {
		if p.home == "" || h.ID == p.home || strings.EqualFold(h.Name, p.home) {
			p.homeID, p.homeName = h.ID, h.Name
			return nil
		}
	}
	if p.home == "" {
		return errors.New("tibber: the account has no homes")
	}
	return fmt.Errorf("tibber: no home %q (found %d homes)", p.home, len(r.Homes))
}

// get calls the Data API with a bearer token; a 401 gets one retry with a fresh token.
func (p *Provider) get(ctx context.Context, u string, out any) error {
	for attempt := 0; ; attempt++ {
		tok, err := p.ts.Token(ctx)
		if err != nil {
			return err
		}
		err = httpx.DoJSON(ctx, p.client, http.MethodGet, u, http.Header{"Authorization": {"Bearer " + tok}}, nil, out)
		if err != nil && attempt == 0 && strings.Contains(err.Error(), "HTTP 401") {
			p.ts.Invalidate()
			continue
		}
		if err != nil && strings.Contains(err.Error(), "HTTP 403") {
			return fmt.Errorf("%w (the OAuth token needs the scopes data-api-homes-read and data-api-prices-read; run tibber-auth again)", err)
		}
		return err
	}
}

// pricesResponse is the subset of GET /v1/homes/{id}/prices we use.
type pricesResponse struct {
	Currency string `json:"currency"`
	Items    []struct {
		Time        string `json:"time"`
		Consumption struct {
			Price flexFloat `json:"price"`
		} `json:"consumption"`
	} `json:"items"`
	Next string `json:"next"`
}

// flexFloat accepts a JSON number or a numeric string (the API schema allows both).
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("bad price %q", s)
	}
	*f = flexFloat(v)
	return nil
}

var _ json.Unmarshaler = (*flexFloat)(nil)
