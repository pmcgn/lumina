package tibber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultTokenURL = "https://thewall.tibber.com/connect/token"

// tokenSource keeps an OAuth2 access token fresh from a refresh token.
// Tibber may rotate the refresh token on every use, so the newest one is
// persisted to a file; the one from the config only seeds that file.
type tokenSource struct {
	client   *http.Client
	tokenURL string
	clientID string
	secret   string
	file     string
	seed     string // refresh token from the config
	now      func() time.Time

	mu      sync.Mutex
	refresh string
	access  string
	expires time.Time
}

// tokenState is the persisted refresh token. Seed remembers which configured
// token it descends from, so a newly issued token in the config wins over a stale file.
type tokenState struct {
	Seed         string `json:"seed"`
	RefreshToken string `json:"refreshToken"`
}

func newTokenSource(client *http.Client, tokenURL, clientID, secret, refresh, file string, now func() time.Time) *tokenSource {
	ts := &tokenSource{client: client, tokenURL: tokenURL, clientID: clientID, secret: secret, file: file, seed: refresh, refresh: refresh, now: now}
	if b, err := os.ReadFile(file); err == nil {
		var st tokenState
		if json.Unmarshal(b, &st) == nil && st.Seed == refresh && st.RefreshToken != "" {
			ts.refresh = st.RefreshToken
		}
	}
	return ts
}

var (
	sourcesMu sync.Mutex
	sources   = map[string]*tokenSource{}
)

// sharedSource returns the token source for an OAuth client, so several
// widgets (or a reload of the config) share one refresh-token chain.
func sharedSource(client *http.Client, tokenURL, clientID, secret, refresh, file string) *tokenSource {
	key := strings.Join([]string{tokenURL, clientID, file, refresh}, "\x00")
	sourcesMu.Lock()
	defer sourcesMu.Unlock()
	if ts, ok := sources[key]; ok {
		return ts
	}
	ts := newTokenSource(client, tokenURL, clientID, secret, refresh, file, time.Now)
	sources[key] = ts
	return ts
}

// Token returns a valid access token, refreshing it when it expires within a minute.
func (ts *tokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.access != "" && ts.now().Add(time.Minute).Before(ts.expires) {
		return ts.access, nil
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {ts.refresh},
		"client_id":     {ts.clientID},
		"client_secret": {ts.secret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "lumina")
	resp, err := ts.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tibber token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &tr)
	if resp.StatusCode/100 != 2 || tr.AccessToken == "" {
		// never echo the body: only the OAuth error code and description
		msg := tr.Error
		if tr.Description != "" {
			msg += ": " + tr.Description
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		if tr.Error == "invalid_grant" {
			msg += " (the refresh token is expired or was already used: run tibber-auth again)"
		}
		return "", fmt.Errorf("tibber token refresh failed: %s", msg)
	}
	ts.access = tr.AccessToken
	ts.expires = ts.now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if tr.RefreshToken != "" && tr.RefreshToken != ts.refresh {
		ts.refresh = tr.RefreshToken
		if err := ts.persist(); err != nil {
			return "", fmt.Errorf("tibber: saving the rotated refresh token to %s failed (it is needed after a restart): %w", ts.file, err)
		}
	}
	return ts.access, nil
}

// Invalidate drops the cached access token (after a 401).
func (ts *tokenSource) Invalidate() {
	ts.mu.Lock()
	ts.access = ""
	ts.mu.Unlock()
}

func (ts *tokenSource) persist() error {
	if ts.file == "" {
		return errors.New("no tokenFile")
	}
	b, err := json.Marshal(tokenState{Seed: ts.seed, RefreshToken: ts.refresh})
	if err != nil {
		return err
	}
	if dir := filepath.Dir(ts.file); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	tmp := ts.file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, ts.file)
}
