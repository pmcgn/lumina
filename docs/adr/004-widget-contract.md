# ADR-004: Widget contract

Status: accepted · Date: 2026-10-17

## Context
Adding an integration must be easy (section 6 of the requirements): one Go package, one frontend component folder, and registration. The core owns scheduling, caching, subscriptions, diffing and error isolation.

## Decision

### Terminology
- **Connection:** configured access to an external system (base URL, token), reusable by many widgets.
- **Widget type:** code (provider + component), e.g. `vikunja.tasks`, `openhab.items`, `weather.forecast`, `calendar.agenda`, `grafana.chart`, `paperless.documents`, `core.clock`.
- **Widget instance:** type + connection + settings + placement on a page.

### Backend (Go) – sketch
```go
type Provider interface {
    Meta() Meta                                // type id, sizes, capabilities, declared actions
    Schema() []byte                            // JSON Schema of the settings (validation)
    Start(ctx context.Context, env Env) error  // env: connection, settings, logger, HTTP client, history store
}

// Optional capabilities, implemented only when needed:
type Poller   interface { Fetch(ctx context.Context) (any, error) }
type Streamer interface { Stream(ctx context.Context, emit func(any)) error }
type Actor    interface { Action(ctx context.Context, name string, params json.RawMessage) (any, error) }

func Register(typeID string, factory func() Provider) // called from the provider package's init()
```
- The core runs one **runner** per active widget instance: scheduling (interval + jitter + backoff), timeout, panic recovery, cache, status (`ok` / `stale` / `error`), diffing and fan-out. Providers never touch WebSockets.
- A runner is active only while at least one client subscribes (FR-18), unless the widget is flagged `always_on` (needed to collect in-memory history, FR-25a).
- Providers return **their own typed, versioned data model** (Go struct), never raw upstream JSON.
- Shared helpers: HTTP/JSON client (auth, timeouts, retries), history store access, downsampling, test harness with a mocked upstream.

### Frontend – sketch
```ts
registerWidget('vikunja.tasks', {
  component: () => import('./VikunjaTasks.svelte'),  // lazy
  sizes: [{ w: 2, h: 2 }, { w: 2, h: 4 }],
  detail: () => import('./VikunjaDetail.svelte'),    // optional
});
// component props: { data, settings, size, status, send(action, params) }
```
- The **widget shell** (card, title, loading/stale/error state, detail opener, undo-toast host) is provided by the framework. Widgets render only their content with design-system components.
- Types for `data` and `settings` are generated from Go (ADR-003).

### Configuration (YAML) – sketch
```yaml
connections:
  vikunja: { type: vikunja, url: https://tasks.example.lan, token: ${VIKUNJA_TOKEN} }
pages:
  - id: home
    title: Home
    widgets:
      - id: tasks-main
        type: vikunja.tasks
        connection: vikunja
        pos: { x: 0, y: 0, w: 2, h: 4 }
        settings: { project: Household, due: all, readOnly: false }
```

### Registration
Compile-time registration (blank imports of provider packages in `cmd/lumina`) and a plain TypeScript registry module on the frontend. No Go `plugin` package and no dynamic loading in v1.

## Alternatives considered
- Dynamic or out-of-process plugins (gRPC/subprocess): flexible but heavy; deferred (WA-13).
- Frontend fetching data itself per widget: breaks A-4 and the single-look/single-credential model.
- Generic "dashboard JSON" widgets only: not expressive enough for actions such as toggling lights.

## Consequences
- Core and widgets are decoupled. A new widget touches only its own folders plus two registration lines.
- Typed models cost some boilerplate but protect the frontend from upstream API changes.
