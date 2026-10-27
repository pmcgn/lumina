# AGENTS.md

Lumina: self-hosted home dashboard for a wall-mounted Android tablet. Go backend polls/streams data and pushes it over one WebSocket to a Svelte 5 SPA. All UI is rendered natively (no iFrames). Read [docs/requirements.md](docs/requirements.md) and [docs/adr/](docs/adr/README.md) before bigger changes.

## Layout

- `backend/` Go module: `cmd/lumina` (server entrypoint), `mocks/demo-services` (mock services for demos, not production code), `internal/{config,core,server,widget,widgets/<type>,history,httpx,sse,web}`
- `frontend/` Svelte 5 + Vite + TypeScript: `src/{lib,components,widgets/<type>,styles,i18n}`
- `config/` example and demo YAML, `deploy/k8s/` manifests, `scripts/demo.ps1`, `Dockerfile`

## Commands

```sh
cd backend  && go vet ./... && go test ./...      # live tests are skipped unless env vars are set
cd backend  && go tool tygo generate               # regenerate frontend/**/*.gen.ts after changing a wire model
cd frontend && npm run check && npm test          # type check + vitest
cd frontend && npm run build                      # writes into backend/internal/web/dist
cd backend  && go build ./cmd/lumina           # embeds the built frontend: rebuild after every frontend build
.\scripts\demo.ps1                                # full demo on :8080, no secrets, nothing real is touched
docker build -t lumina:dev .
```

## Adding a widget

Backend: package in `internal/widgets/<name>` implementing `widget.Provider` (+ `Poller`/`Streamer`/`Actor`), `widget.Register` in `init()`, blank import in `cmd/lumina/main.go`; the data sent to the browser and a `Settings` struct go in `model.go` (settings are decoded strictly with `widget.DecodeSettings`; implement `widget.Validator` so the config loader rejects bad settings), with a package block in `backend/tygo.yaml`. Frontend: component in `src/widgets/<name>/`, a `types.ts` that re-exports the generated `model.gen.ts`, and one line in `src/widgets/index.ts`. Add i18n keys to both `de.json` and `en.json`. See ADR-004 and ADR-007.

## Rules and gotchas

- **Browser baseline is Chrome 80** (older Android tablets): no `color-mix()`, `:has()`, container queries; use the tokens in `tokens.css`. Hover styles go in `@media (hover: hover)`.
- **Secrets stay in `connections`**, referenced by name; settings reach the browser, connections never. Never log or echo feed URLs or tokens. Pass tokens via `${ENV}` in YAML (full-line comments are not expanded).
- **No login (LAN only)**: write actions are validated server-side against what the widget rendered, and every widget supports `readOnly`.
- **Production systems** (the user's openHAB, Vikunja): read-only unless told otherwise; prefer `demo-services`. Delete any test data you create. Live tests need `VIKUNJA_URL/TOKEN`, `OPENHAB_URL/SITEMAP`, `CALENDAR_URL`, `GRAFANA_URL/TOKEN/DASHBOARD/PANELS`.
- **Types are generated, not copied** (ADR-007): wire models live in `model.go` (per widget), `config/client.go` and `core/protocol.go`; tygo turns them into `*.gen.ts`. Never edit those by hand; run `go tool tygo generate` and commit the result (the test in `internal/typegen` fails if they are stale). String enums need Go consts prefixed with the type name (`ConditionRain`) to become TypeScript unions. Widget settings are typed Go structs too (`Settings` in `model.go`, flexible YAML values via `widget.StringList` / `widget.FlexString`), so they are generated as well; a test checks that every widget type validates its settings strictly.
- Keep frontend widgets dumb: data comes from the backend model, formatting and relative times are done in the browser (see `lib/dates.ts`, `widgets/calendar/agenda.ts`), with unit tests for date logic.
- Use `svelte-check` with TypeScript 5.x (7 is unsupported). Don't `taskkill msedge.exe` (closes the user's browser); use a separate `--user-data-dir` for headless screenshots.
- Write Go files with the file tools: generating Go strings containing backslashes through shell/Python heredocs gets mangled.
- Nothing is committed yet (no git repo): do not commit unless asked.
