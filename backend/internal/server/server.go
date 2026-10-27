// Package server contains the HTTP layer: WebSocket endpoint, health probes
// and the embedded frontend.
package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/pmcgn/lumina/internal/core"
)

const (
	pingInterval = 15 * time.Second
	writeTimeout = 5 * time.Second
	readLimit    = 64 << 10
)

type Server struct {
	hub *core.Hub
	log *slog.Logger
	web fs.FS

	mu      sync.RWMutex
	origins []string
}

func New(hub *core.Hub, log *slog.Logger, web fs.FS) *Server {
	return &Server{hub: hub, log: log, web: web}
}

// SetAllowedOrigins sets additional allowed WebSocket origin host patterns.
func (s *Server) SetAllowedOrigins(o []string) {
	s.mu.Lock()
	s.origins = o
	s.mu.Unlock()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !s.hub.Ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ws", s.serveWS)
	mux.Handle("/", s.static())
	return mux
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	origins := s.origins
	s.mu.RUnlock()

	// Accept enforces same-origin unless the Origin matches an allowed pattern.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: origins})
	if err != nil {
		s.log.Warn("websocket upgrade rejected", "remote", r.RemoteAddr, "err", err)
		return
	}
	conn.SetReadLimit(readLimit)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	client := s.hub.NewClient(cancel)
	defer s.hub.RemoveClient(client)
	log := s.log.With("remote", r.RemoteAddr)
	log.Info("client connected")
	defer log.Info("client disconnected")

	go writeLoop(ctx, cancel, conn, client)

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		var msg core.Envelope
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Warn("bad message", "err", err)
			continue
		}
		s.hub.Handle(client, msg)
	}
	conn.Close(websocket.StatusGoingAway, "")
}

func writeLoop(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, c *core.Client) {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-c.Out():
			wctx, wcancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Write(wctx, websocket.MessageText, msg)
			wcancel()
			if err != nil {
				cancel()
				return
			}
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, 2*pingInterval)
			err := conn.Ping(pctx)
			pcancel()
			if err != nil {
				cancel()
				return
			}
		}
	}
}

// static serves the embedded SPA; unknown paths fall back to index.html.
func (s *Server) static() http.Handler {
	files := http.FileServer(http.FS(s.web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.web, p); err != nil {
			r.URL.Path = "/"
			p = "index.html"
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
