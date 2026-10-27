package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pmcgn/lumina/internal/config"
	"github.com/pmcgn/lumina/internal/core"
	"github.com/pmcgn/lumina/internal/server"
	"github.com/pmcgn/lumina/internal/web"

	// Widget providers register themselves via init().
	_ "github.com/pmcgn/lumina/internal/widgets/calendar"
	_ "github.com/pmcgn/lumina/internal/widgets/clock"
	_ "github.com/pmcgn/lumina/internal/widgets/grafana"
	_ "github.com/pmcgn/lumina/internal/widgets/openhab"
	_ "github.com/pmcgn/lumina/internal/widgets/vikunja"
	_ "github.com/pmcgn/lumina/internal/widgets/weather"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

const reloadPollInterval = 2 * time.Second

func main() {
	cfgPath := flag.String("config", env("LUMINA_CONFIG", "/config/config.yaml"), "path to the YAML configuration")
	addr := flag.String("addr", env("LUMINA_ADDR", ":8080"), "listen address")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel()}))
	slog.SetDefault(log)
	log.Info("starting lumina", "version", version, "config", *cfgPath, "addr", *addr)

	cfg, raw, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("cannot load configuration", "err", err)
		os.Exit(1)
	}

	// Clients reload when this changes, so it also has to change when only the
	// frontend was rebuilt while the -ldflags version stayed "dev".
	frontend := web.FS()
	fullVersion := version
	if id := web.BuildID(frontend); id != "" {
		fullVersion += "+" + id
	}
	hub := core.NewHub(log, fullVersion)
	srv := server.New(hub, log, frontend)
	srv.SetAllowedOrigins(cfg.Server.AllowedOrigins)
	hub.Apply(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go watchConfig(ctx, log, *cfgPath, raw, hub, srv)

	httpSrv := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		log.Info("shutting down")
		hub.Close() // disconnects WebSocket clients (hijacked connections are not covered by Shutdown)
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpSrv.Shutdown(sctx)
	}()

	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
}

// watchConfig polls the file content. Polling (instead of inotify) also works
// for Kubernetes ConfigMap mounts, which are updated by swapping symlinks.
func watchConfig(ctx context.Context, log *slog.Logger, path string, last []byte, hub *core.Hub, srv *server.Server) {
	t := time.NewTicker(reloadPollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		cur, err := os.ReadFile(path)
		if err != nil || bytes.Equal(cur, last) {
			continue
		}
		last = cur
		cfg, err := config.Parse(cur, os.LookupEnv)
		if err != nil {
			log.Error("configuration reload failed, keeping the previous one", "err", err)
			continue
		}
		srv.SetAllowedOrigins(cfg.Server.AllowedOrigins)
		hub.Apply(cfg)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func logLevel() slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(os.Getenv("LUMINA_LOG_LEVEL"))); err != nil {
		return slog.LevelInfo
	}
	return l
}
