# Lumina

A self-hosted, responsive home dashboard for a wall-mounted tablet. A Go backend polls your services and pushes live updates over one WebSocket to a lightweight Svelte frontend. Everything is rendered natively (no iFrames), via widgets.

- Requirements: [docs/requirements.md](docs/requirements.md)
- Architecture decisions: [docs/adr/](docs/adr/README.md)

## Status

- M0 (foundations): done. Server, WebSocket protocol, config with hot reload, frontend shell, widget registry, container build.
- M1 (first integration): weather via Open-Meteo (`weather.forecast`), the in-memory history store (`backend/internal/history`) and the native SVG chart component. Done.
- M2 (first widget with actions): Vikunja (`vikunja.tasks`): open tasks with due dates, overdue highlighted and sorted first, complete with undo, quick add. Done.
- M3 (live streaming + control): openHAB (`openhab.sitemap`): a sitemap shown natively, live via the event stream, switches and mapping buttons operable. Done.
- M4 (dashboard features): openHAB sliders (dimmers, blinds with up/stop/down, numeric setpoints), page auto-rotation, night mode (dim or clock), nightly soft reload, lite mode. Done.
- M5 (calendar): `calendar.agenda` merges iCal feeds (Google secret address and others): recurrence, time zones, all-day events, agenda plus month overview. Done.
- M6 (Grafana and typed settings): `grafana.panel` draws a panel of an existing dashboard as chart, stat or gauge through Grafana's query API (any datasource). All widget settings are now typed Go structs: validated strictly when the config is loaded (a typo is an error that lists the valid settings) and generated to TypeScript. Done.
- Next: Paperless-ngx; color pickers and selections for openHAB; a week view for the calendar.

Widgets: `calendar.agenda`, `core.clock`, `grafana.panel`, `openhab.sitemap`, `vikunja.tasks`, `weather.forecast` (a card adapts to its size: compact shows current conditions, taller cards add the 24 h chart and the 7-day forecast).

## Layout

```
backend/    Go module (cmd/lumina, internal/{config,core,server,widget,widgets/<type>,web})
frontend/   Svelte 5 + Vite SPA (src/{lib,components,widgets/<type>,styles,i18n})
config/     example configuration
deploy/k8s/ example Kubernetes manifests
scripts/    demo.ps1 (builds and starts the all-in-one demo)
```

## Develop

Needs Go 1.24+ and Node 22+.

```sh
# 1. backend (serves the embedded frontend and /ws on :8080)
cd backend
go run ./cmd/lumina -config ../config/config.example.yaml

# 2. frontend dev server with hot reload (proxies /ws to :8080)
cd frontend
npm install
npm run dev        # http://localhost:5173
```

For the dev server the WebSocket origin must be allowed: add `server: { allowedOrigins: ["localhost:5173"] }` to your config.

Build everything into one binary:

```sh
cd frontend && npm run build      # writes into backend/internal/web/dist
cd ../backend && go build -o lumina ./cmd/lumina
```

Types shared with the frontend are generated from the Go data models (`model.go` per widget, `config/client.go`, `core/protocol.go`) into `frontend/**/*.gen.ts` by [tygo](https://github.com/gzuidhof/tygo). After changing a model, run this in `backend/` and commit the result:

```sh
go tool tygo generate
```

Tests and checks:

```sh
cd backend && go test ./...
cd frontend && npm run check
```

## Display behaviour

The optional `display` section of the config controls the always-on screen: page `rotate` (a touch pauses it), a `night` window with `dim` or `clock` mode (a touch wakes the clock), a daily `softReload` (default 03:30) as a safety net for 24/7 operation, and `lite` mode without animations. See [config/config.example.yaml](config/config.example.yaml). Night times use the tablet's own clock. Night mode is software only: it does not control the real screen or its brightness.

## Demo

`scripts/demo.ps1` (Windows PowerShell) builds everything and starts a demo with **every integration** (page 1: clock, weather, calendar, Vikunja tasks, openHAB; page 2: Grafana as chart, gauge and stat) using [config/config.demo.yaml](config/config.demo.yaml). It needs no secrets and touches none of your real systems:

- `backend/mocks/demo-services` is a stand-in for openHAB (sitemap `Demo`: lights, dimmer, blinds, heating mode, live values), Vikunja (token `demo`; complete, undo, quick add, a recurring task), Grafana (token `demo`; a dashboard with generated household curves) and two iCal feeds, all with dates relative to now. State lives in memory; restart to reset.
- Weather is live from Open-Meteo, so you need an internet connection. The calendar also includes Google's public holiday calendar.

```powershell
.\scripts\demo.ps1                 # http://localhost:8080, opens the browser, Ctrl+C stops everything
.\scripts\demo.ps1 -Port 9000 -NoBrowser -SkipBuild
```

Without PowerShell: `go run ./mocks/demo-services` (in `backend/`) and `go run ./cmd/lumina -config ../config/config.demo.yaml`, after `npm run build` in `frontend/`.

To try your own systems, copy the demo config and replace the `connections` (see the comments in the file).

## Container

```sh
docker build -t lumina --build-arg VERSION=0.1.0 .
docker run --rm -p 8080:8080 -v "$PWD/config/config.example.yaml:/config/config.yaml:ro" lumina
```

Kubernetes: see [deploy/k8s/lumina.yaml](deploy/k8s/lumina.yaml). Keep one replica. The dashboard has no login and is meant for the LAN only; do not expose it to the internet. Use `readOnly: true` on widgets that must not control anything.

## Configuration

Settings are validated strictly when the file is loaded: an unknown or mistyped setting (say `dueTo` instead of `due`) or a wrong value is an error that names the widget and lists the valid settings, and the previously loaded configuration stays active.

See [config/config.example.yaml](config/config.example.yaml). The file is watched and reloaded automatically (also for ConfigMap mounts). An invalid edit is logged and ignored; the last good configuration stays active. `${VAR}` references are replaced by environment variables before parsing.

## Adding a widget

1. Backend: a package in `backend/internal/widgets/<name>` that implements `widget.Provider` (plus `Poller`/`Streamer`/`Actor` as needed) and calls `widget.Register` in `init()`; add a blank import in `cmd/lumina/main.go`.
2. Frontend: a component in `frontend/src/widgets/<name>/` and one `registerWidget` line in `frontend/src/widgets/index.ts`.
3. Use it in the config under `pages[].widgets[]`.

See [ADR-004](docs/adr/004-widget-contract.md) for the contract.
