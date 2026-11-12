package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
)

const (
	webhookBodyLimit = 1 << 20 // task payloads can be large (descriptions, attachments)
	vikunjaType      = "vikunja.tasks"
	vikunjaSignature = "X-Vikunja-Signature"
)

// vikunjaWebhook is the receiver for Vikunja webhooks:
// POST /api/webhooks/vikunja/{connection}. Every event only means "something
// changed": the payload is not used, the widgets using the connection fetch the
// task list again. That keeps one source of truth (no task can show up twice,
// lost or reordered deliveries do not matter, the regular poll stays the safety
// net).
//
// Authentication is optional but enforced once configured, in the connection:
//   - webhookUser + webhookPassword: HTTP basic auth is required,
//   - webhookSecret: the X-Vikunja-Signature header (hex HMAC-SHA256 of the raw
//     body, keyed with the webhook secret) is required.
//
// If both are configured, both must pass.
func (s *Server) vikunjaWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("connection")
	conn, ok := s.hub.Connection(name)
	if !ok {
		http.Error(w, "unknown connection", http.StatusNotFound)
		return
	}
	// Never log the URL's secrets or the body; the connection name is not secret.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookBodyLimit))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "cannot read body", http.StatusBadRequest)
		}
		return
	}

	if status, msg := authorizeVikunjaWebhook(r, conn, body); status != 0 {
		if status == http.StatusUnauthorized && msg == "basic" {
			w.Header().Set("WWW-Authenticate", `Basic realm="lumina webhook", charset="UTF-8"`)
		}
		s.log.Warn("webhook rejected", "connection", name, "remote", r.RemoteAddr, "reason", msg)
		http.Error(w, http.StatusText(status), status)
		return
	}

	n := s.hub.RefreshConnection(name, vikunjaType)
	s.log.Debug("webhook accepted", "connection", name, "widgets", n)
	w.WriteHeader(http.StatusNoContent)
}

// authorizeVikunjaWebhook checks the credentials configured in the connection.
// It returns status 0 when the request may pass, otherwise the HTTP status and a
// short reason for the log (never the credentials).
func authorizeVikunjaWebhook(r *http.Request, conn map[string]any, body []byte) (int, string) {
	user, _ := conn["webhookUser"].(string)
	pass, _ := conn["webhookPassword"].(string)
	secret, _ := conn["webhookSecret"].(string)

	if user != "" || pass != "" {
		u, p, ok := r.BasicAuth()
		if !ok {
			return http.StatusUnauthorized, "basic"
		}
		// Compare both always, and hashed so the lengths do not leak.
		okUser := equalSecret(u, user)
		okPass := equalSecret(p, pass)
		if !okUser || !okPass || user == "" || pass == "" { // half-configured: fail closed
			return http.StatusUnauthorized, "basic"
		}
	}
	if secret != "" {
		if !validSignature(secret, body, r.Header.Get(vikunjaSignature)) {
			return http.StatusUnauthorized, "signature"
		}
	}
	return 0, ""
}

func equalSecret(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// validSignature checks a hex HMAC-SHA256 of body (an optional "sha256=" prefix
// is tolerated).
func validSignature(secret string, body []byte, header string) bool {
	header = strings.TrimPrefix(strings.TrimSpace(header), "sha256=")
	got, err := hex.DecodeString(header)
	if err != nil || len(got) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
