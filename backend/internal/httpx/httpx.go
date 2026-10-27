// Package httpx is the shared helper for providers that talk JSON over HTTP.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxBody = 8 << 20 // 8 MiB

// NewClient returns an HTTP client with a sane overall timeout.
func NewClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// GetJSON GETs url and decodes the JSON response into out. Non-2xx responses
// and oversized bodies are errors.
func GetJSON(ctx context.Context, c *http.Client, url string, out any) error {
	return DoJSON(ctx, c, http.MethodGet, url, nil, nil, out)
}

// DoJSON sends an optional JSON body with the given method and headers and
// decodes the JSON response into out (if non-nil).
func DoJSON(ctx context.Context, c *http.Client, method, url string, header http.Header, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "lumina")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(data) > maxBody {
		return fmt.Errorf("response from %s too large", req.URL.Host)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, req.URL.Host, resp.StatusCode, snippet(data))
	}
	if out == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%s: decode: %w", req.URL.Host, err)
	}
	return nil
}

func snippet(b []byte) string {
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}

// PostText sends a plain-text body (e.g. an openHAB item command) and returns
// once the server answered with a 2xx status.
func PostText(ctx context.Context, c *http.Client, url string, header http.Header, body string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("User-Agent", "lumina")
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("POST %s: HTTP %d: %s", req.URL.Host, resp.StatusCode, snippet(data))
	}
	return nil
}
