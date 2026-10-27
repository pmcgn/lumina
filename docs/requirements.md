# Lumina – Requirements

Status: Draft v0.4 (2026-10-17) – all initial open questions answered (section 11)

A self-hosted, responsive home dashboard that runs as a website on a (possibly older) tablet. All content is rendered natively by the application. Data from external systems is shown through pluggable widgets.

Keywords: **MUST** = required, **SHOULD** = desired, **MAY** = optional / later.

---

## 1. Goals and Non-Goals

### 1.1 Goals
- G1: One beautiful, consistent, glanceable dashboard for the home, shown permanently on a tablet.
- G2: Live data: widgets update automatically without user interaction or page reload.
- G3: Uniform look & feel across all widgets (no foreign UI, no iFrames).
- G4: Smooth performance on old tablets (see 4.1).
- G5: Easy to add new integrations (widgets) without touching the core.

### 1.2 Non-Goals (initial)
- Replacing the source systems (openHAB, Vikunja, ...). Lumina is primarily a viewer with light interaction.
- Multi-tenant / public internet hosting.
- Native mobile apps (a PWA-style website is sufficient).

---

## 2. Users and Usage Context

- U1: Primary user is the household; one admin (the owner) configures the system.
- U2: Main device: wall-mounted or standing tablet, always on, browser in fullscreen/kiosk mode, touch input. Landscape is primary, portrait SHOULD work.
- U3: Secondary devices: phone and desktop browser (layout MUST adapt, but tablet is the design target).
- U4: Runs in the home LAN; remote access (VPN/reverse proxy) is out of scope but MUST not be prevented.

---

## 3. Architecture Overview

```
 Tablet browser (SPA)  <--- WebSocket --->  Go backend  <--- plugins --->  vikunja, openHAB,
   widgets (frontend)                        widget runtime                  weather, grafana,
                                             cache, poll scheduler           paperless-ngx, calendar, ...
```

- A-1: **Backend in Go**, single static binary (plus config/data dir), also serves the built frontend assets.
- A-2: **Frontend** is a single-page app built with a modern framework (see 4.1 for constraints; candidates: Svelte, Preact, Solid – decision to be made in an ADR).
- A-3: **All frontend ⇄ backend communication runs over WebSockets** (data, updates, widget actions). Only exceptions: initial static asset delivery, and optional health/metrics endpoints.
- A-4: **The frontend never talks to third-party systems directly.** The backend is the only component with credentials and network access to integrations (security, CORS, one look).
- A-5: **Delivered as a container image** (decision Q9), hosted in the owner's **Kubernetes cluster**. Single small image (multi-stage build, minimal/distroless base, non-root, read-only root filesystem). SHOULD support linux/amd64 and linux/arm64. The repo ships example Kubernetes manifests (Deployment, Service, ConfigMap, Secret reference, Ingress) and MAY ship a Helm chart later; a plain binary run for development is a side effect, not a goal.
- A-6: **Kubernetes-friendly runtime:** configuration from a mounted ConfigMap (YAML, hot reload must cope with ConfigMap symlink swaps) and secrets from env vars/mounted Secrets; `/healthz` (liveness) and `/readyz` (readiness) endpoints; graceful shutdown on SIGTERM (close WebSockets cleanly); logs to stdout; **single replica**, stateless apart from the in-memory data (no persistent volume needed); modest resource requests/limits (target well below ~100 MB RAM and low CPU at idle). Ingress MUST support WebSocket upgrade and long-lived connections (document required annotations/timeouts).

---

## 4. Non-Functional Requirements

### 4.1 Performance & Compatibility (older tablets)
- NFR-1: MUST run smoothly on older **Android tablets with 4 GB RAM** (decision Q1). The exact model/WebView version is unknown, so the app MUST be tested on the real device early (M0) and the browser baseline adjusted if needed.
- NFR-2: Frontend build MUST target a conservative browser baseline (assume an Android System WebView/Chrome of ~v80+ as default; lower it if the real tablet requires, see Q1), with transpilation and only polyfills that are needed.
- NFR-3: Small bundle: initial JS SHOULD be < 150 KB gzipped (excluding lazy-loaded widget code); widget code SHOULD be lazy-loaded.
- NFR-4: Animations MUST use GPU-friendly properties only (transform, opacity); no layout-thrashing. Animations MUST be disableable/reducible (config + `prefers-reduced-motion`).
- NFR-5: Avoid heavy visual effects (large blur/backdrop-filter, big shadows) or provide a "lite" rendering mode switch.
- NFR-6: Updates MUST be fine-grained: only changed widgets/values re-render. Charts MUST be rendered with light-weight technology (SVG/Canvas, own or small library); no heavy chart libs unless proven fast on the baseline device.
- NFR-7: Memory MUST stay stable over weeks of uptime (no leaks; the page runs 24/7). A scheduled soft-reload (e.g. nightly) SHOULD be available as safety net.
- NFR-8: Time to first meaningful paint on LAN SHOULD be < 3 s on baseline device.

### 4.2 Reliability & Resilience
- NFR-9: Frontend MUST auto-reconnect the WebSocket with exponential backoff + jitter and show an unobtrusive connection-state indicator.
- NFR-10: On reconnect the backend MUST send the current state snapshot of all subscribed widgets (no stale/missed updates).
- NFR-11: One failing integration MUST NOT affect other widgets (isolation, timeouts, panic recovery per widget instance).
- NFR-12: Widgets MUST show last known data with a "stale since …" marker when the source is unreachable, and a clear error state otherwise.
- NFR-13: Backend MUST survive restarts without losing configuration (it comes from the ConfigMap). **No persistence in v1 (decision Q10):** caches and history are in memory only and are lost on restart/redeploy; after a restart widgets refill from the sources and history charts start empty and build up again.
- NFR-14: The frontend MUST pick up new backend versions automatically (version handshake over WS → reload).

### 4.3 Security
- NFR-15: Secrets (API tokens, passwords) are stored only on the backend (env vars / secrets file / config with restricted permissions) and are NEVER sent to the browser.
- NFR-16: **No login** (decision Q4): the dashboard is LAN-only and unauthenticated. The server MUST bind to a configurable address, MUST NOT be exposed to the internet by default and the docs MUST warn about this. Because there is no auth, **write actions are protected by per-widget/connection read-only flags** (FR-31) and an optional allowlist of client IPs/networks. Auth (e.g. PIN, reverse-proxy) MAY be added later; keep the design open for it.
- NFR-17: WebSocket MUST validate the Origin header (same-origin) to prevent cross-site WebSocket hijacking from other websites in the LAN browser; TLS via reverse proxy is optional.
- NFR-18: Outbound requests to integrations MUST have timeouts, size limits and be restricted to configured hosts.
- NFR-19: Documents/images proxied from sources (e.g. Paperless) MUST pass through the backend with auth checks and MUST NOT expose upstream credentials.

### 4.4 Maintainability & Quality
- NFR-20: Clear module boundaries; widget plugins depend on the core API, never the reverse.
- NFR-21: Unit tests for core + each widget's backend provider (with mocked upstream); frontend component tests for the widget framework; at least one E2E smoke test.
- NFR-22: Structured logging (leveled), configurable via env/config.
- NFR-23: Everything needed to run is documented (README, config reference, widget authoring guide).
- NFR-24: Language/locale: UI texts externalised (i18n); **German and English** from v1 (decision Q7), switchable per device/config, default German. Dates, numbers and units follow the selected locale and a configurable timezone. Backend-provided texts (e.g. weather descriptions) are delivered as codes and translated in the frontend.

---

## 5. Functional Requirements – Core

### 5.1 Dashboard & Layout
- FR-1: The dashboard consists of one or more **pages**/screens, each containing a grid of **widget instances**.
- FR-2: Grid layout with configurable position and size (columns × rows) per widget; responsive breakpoints for tablet landscape/portrait, phone, desktop.
- FR-3: Navigation between pages via swipe and/or tab bar; optional **auto-rotation** of pages with configurable interval and pause on touch.
- FR-4: Widgets MAY offer multiple size variants (e.g. 1×1 compact, 2×2 detail) and render accordingly (container-size-based, not only viewport-based).
- FR-5: Widgets MAY open a **detail view / modal** (full screen or overlay) with more information and interaction, rendered natively.
- FR-6: Layout configuration MUST be persisted on the backend (file-based, e.g. YAML/JSON) and shared across devices.
- FR-7: Layout and widgets are configured **via YAML file with hot reload** (decision Q5). A visual edit mode is out of scope for v1, but the config model SHOULD NOT preclude it later.
- FR-8: Configuration MUST be validated on load with helpful error messages; invalid config MUST NOT take down the running dashboard (keep last good config).

### 5.2 Theming & Design
- FR-9: One consistent design system: shared tokens (colors, spacing, radius, typography, elevation, motion), shared base components (card, list, stat, chart, icon, button, badge, empty/error/loading state).
- FR-10: **Light/dark theme** plus automatic switching (by time/sunset or system); configurable accent color.
- FR-11: Large, touch-friendly targets (≥ 44 px), readable from a distance (large type for key values), no hover-dependent interactions.
- FR-12: **Ambient / night mode:** software-only dimming (dark overlay) or clock-only screen on schedule, wake on touch; the tablet's real screen/brightness is not controlled (Q11). SHOULD avoid burn-in (slow pixel shifting).
- FR-13: Fonts and icons are bundled locally (no external CDN calls – works offline in LAN, privacy).
- FR-14: Fullscreen/kiosk friendly: no browser chrome dependency, prevent accidental zoom/selection, optional screen wake-lock requested from the page (browser API only; **no device/tablet control** in v1, decision Q11).
- FR-15: Installable as PWA (manifest, icons) SHOULD; offline shell with "connection lost" state.

### 5.3 Real-time Communication (WebSocket protocol)
- FR-16: Single multiplexed WebSocket per client. Message envelope is versioned JSON (`{v, type, id, widgetId, payload, ts}`); a compact encoding (e.g. MessagePack) MAY be added later.
- FR-17: Message types (minimum): `hello/auth`, `config` (layout + widget definitions), `subscribe/unsubscribe` (per widget instance), `snapshot`, `update` (full or patch), `action` (client → backend widget command) + `action_result`, `error`, `ping/pong`, `version`.
- FR-18: **Subscription-based data flow:** the backend only polls/streams data for widgets that are currently displayed by at least one connected client (or that are marked "always on"). Shared data fetched once and fanned out to all clients.
- FR-19: Backend pushes updates only when data actually changed (diffing/ETag-style); updates SHOULD be coalesced/throttled to avoid UI flooding.
- FR-20: Multiple simultaneous clients MUST be supported (e.g. tablet + phone), at least ~10 without noticeable load.
- FR-21: Heartbeats detect dead connections within ~30 s on both sides.
- FR-22: A relative-time ticker (clocks, "5 min ago") runs in the frontend and does NOT require server traffic.

### 5.4 Data Acquisition in the Backend
- FR-23: Each widget provider declares how it gets data: **poll** (interval, jitter, backoff on error), **stream/push** (e.g. SSE/WebSocket/webhook from source), or **hybrid**.
- FR-24: Central **scheduler** with per-provider intervals, concurrency limits, timeouts, retries with backoff, and rate-limit awareness.
- FR-25: Central **cache** (in-memory, TTL, last-known-good) with stale marking. No persistence in v1.
- FR-25a: **In-memory history ("recent values")** (decision Q10): the core provides a small, generic time-series store (ring buffers per series) that providers can write to, so charts such as the last 24 h work for sources that only expose current values (e.g. openHAB items, Grafana-less metrics).
  - Retention default **24 h**, configurable per series; fixed, small memory budget (global cap on series count and points, e.g. ≤ ~20 MB total); storage resolution is fixed at one point per series per minute (so a 24 h series is at most 1440 points), and queries are **downsampled** to the chart width (LTTB) so the frontend never receives more points than it can draw.
  - Providers that can query real history from the source (Grafana/Prometheus/InfluxDB, openHAB persistence, weather forecast APIs) SHOULD **backfill** the series from the source on start/subscribe, so charts are full immediately after a restart. The local store is then only a fallback for sources without history.
  - Charts MUST handle partially filled series gracefully (e.g. "collecting data since 14:05", no misleading axis).
  - If the in-memory approach later proves insufficient (longer retention, survive restarts), persistence (e.g. SQLite on a PVC) can be added behind the same store interface; the interface MUST therefore be abstract.
- FR-26: Providers are configured once as **connections** (e.g. one openHAB server, one Vikunja instance) and reused by multiple widget instances; multiple connections of the same type MUST be supported.
- FR-27: Inbound webhooks MAY be offered so sources can push changes (e.g. Vikunja, Paperless) instead of polling.
- FR-28: Basic observability: `/healthz`, per-provider status (last success, last error, latency) visible in an admin/status view; Prometheus metrics endpoint MAY be offered.

### 5.5 Interactions / Actions
- FR-29: The dashboard is **mainly for display, with selected controls** (decision Q8): e.g. switch lights in openHAB, complete tasks in Vikunja. Widgets MAY define **actions** executed by the backend against the source (e.g. toggle a light, complete a task). Actions are explicit, typed and permission-checked.
- FR-30: UI gives immediate feedback (optimistic state or pending state) and reconciles with the real state from the source.
- FR-31: Each widget/connection MUST be configurable as **read-only** (controls then hidden or disabled). Since there is no login (NFR-16), this is the main safeguard. Hard-to-undo actions SHOULD offer a short "Undo" toast instead of confirmation dialogs.

---

## 6. Widget Architecture (extensibility)

The most important design requirement: adding a new integration MUST be possible by adding one backend provider package and one frontend widget package, without modifying core code beyond a registration line (or no change at all if auto-discovery is used).

### 6.1 Concept
A **widget type** consists of two halves joined by a contract:
1. **Backend provider (Go):** fetches/streams data, normalises it, handles actions, validates its own config.
2. **Frontend widget (component):** renders the normalised data using the shared design system.

A **widget instance** = widget type + settings + connection reference + layout position.

### 6.2 Backend contract (Go)
- WA-1: A Go interface for providers, roughly:
  - `Meta()` – type id, name, version, supported sizes, capabilities (poll/stream/actions).
  - `ConfigSchema()` – declarative settings schema (JSON Schema) used for validation and, later, for auto-generated settings forms.
  - `Init(ctx, connection, settings)` / `Close()` lifecycle.
  - `Fetch(ctx) (Data, error)` for polling and/or `Stream(ctx, emit func(Data))` for push sources.
  - `HandleAction(ctx, name, params) (Result, error)` (optional).
- WA-2: Providers are registered in a **registry** (compile-time registration via `init()`/explicit register; Go plugin system is not required). Built-in providers live in separate packages (e.g. `internal/widgets/vikunja`).
- WA-3: The core handles scheduling, caching, subscription, fan-out, diffing, error isolation and reconnect logic – providers only implement fetching/mapping. Providers MUST NOT manage WebSockets themselves.
- WA-4: Data returned by providers is a **typed, versioned, JSON-serialisable model per widget type** (not raw upstream payloads) so that the frontend is decoupled from upstream API changes.
- WA-5: Provider tests MUST be possible with a mocked HTTP upstream and without the rest of the system (test harness/helpers in the core).
- WA-6: A generic **"HTTP/JSON" connector helper** (auth, base URL, TLS options, timeouts, retry) is provided to reduce provider boilerplate.

### 6.3 Frontend contract
- WA-7: Each widget type exports a component with a standard prop interface: `data`, `settings`, `size`, `status` (loading / ok / stale / error), `send(action, params)`.
- WA-8: Widgets are registered in a **frontend widget registry** (type id → lazy-loaded component, default size, optional detail view, optional settings form).
- WA-9: The framework supplies the widget **shell** (card chrome, title, loading/error/stale states, refresh indicator, detail-view open) so individual widgets only render content and look uniform automatically.
- WA-10: Widgets use only shared design-system components and tokens; custom CSS is scoped to the widget. No external embeds, iFrames, or third-party UI.
- WA-11: Type definitions for widget data models are shared/generated from the Go models (e.g. JSON Schema → TS types) to avoid contract drift.

### 6.4 Adding a widget – acceptance test
A developer can add a new example widget (e.g. "Hello/Clock") by: (1) one Go package implementing the provider, (2) one frontend component folder, (3) registering both, (4) adding an entry to the config – in under ~1 hour, guided by documentation and a widget template/scaffold (a `make new-widget` generator MAY be provided).

### 6.5 Generic / escape-hatch widgets (later)
- WA-12: MAY provide generic widgets (e.g. "generic REST/JSON value", "generic MQTT value", "Markdown/text", "image URL via backend proxy") to cover small needs without writing code.
- WA-13: MAY support externally provided providers (e.g. subprocess/HTTP/gRPC plugin) for non-Go integrations – not required for v1.

---

## 7. Initial Widget Set

Common to all: configurable via connection + settings, read-only mode, compact + detail view where useful, stale/error states, auto-update.

### 7.1 Clock / Date (built-in, no external source)
- W-CLK-1: Time, date, week number; optional world clocks. Basic widget to validate the framework; runs offline.

### 7.2 openHAB
- W-OH-1: Connect via openHAB REST API (token auth); use SSE event stream for live item updates (fallback to polling).
- W-OH-2: Show selected **items** or groups: switches, dimmers, color, rollershutters, numbers (temperature, power…), strings, contacts/sensors, with item type–appropriate controls and state display.
- W-OH-3: Control: **switch lights on/off** with one tap (primary use case), plus dim, set value, open/close – respecting read-only mode. The UI shows a pending state until openHAB confirms the new state via the event stream and reverts on failure/timeout.
- W-OH-4: Items are configured by name; label, icon and unit are taken from openHAB metadata but overridable. Grouping into "rooms"/sections.
- W-OH-5: SHOULD support scene/rule/trigger buttons and a compact "status summary" (e.g. "3 lights on, 1 window open").
- W-OH-6: SHOULD show item history sparklines/charts (last 24 h): backfilled from openHAB persistence if available, otherwise from the in-memory history store (FR-25a).

### 7.3 Weather
- W-WX-1: Current conditions, hourly forecast, multi-day forecast, location configurable (lat/lon or name).
- W-WX-2: **Open-Meteo** is the v1 data source (decision Q3: free, no API key, good for Germany). It sits behind a provider abstraction so sources (e.g. Bright Sky / DWD, OpenWeatherMap) can be added later. Data is normalised to Lumina's own weather model (condition codes, not upstream texts).
- W-WX-3: Optional: precipitation, wind, UV, sunrise/sunset, weather warnings (DWD), air quality. Uses locally bundled icon set and unit/locale preferences.
- W-WX-4: Provider data cached with long TTL (e.g. 10–15 min) respecting API limits.

### 7.4 Calendar (Google / iCal)
- W-CAL-1: v1 supports **iCal/ICS URLs** (incl. recurrence rules RRULE, exceptions, time zones, all-day events), which covers Google Calendar via its **secret iCal address** (decision Q2). Google OAuth/API and CalDAV are out of scope for v1, but the provider SHOULD be structured so a second source type can be added.
- W-CAL-2: Multiple calendars with color per calendar, merged in one view.
- W-CAL-3: Views: agenda/upcoming list, day, week, and month overview (size dependent); highlight current/next event; birthdays/holidays styling.
- W-CAL-4: Read-only in v1 (event creation MAY follow).
- W-CAL-5: Server-side parsing, expansion of recurrences within the requested window, and caching; refresh interval configurable (default 5–15 min).

### 7.5 Vikunja (tasks)
The widget is a **working task list, not a counter** (unlike e.g. gethomepage.dev, which only shows the number of open tasks).
- W-VK-1: Connect via Vikunja API token.
- W-VK-2: **List the currently open tasks** individually, each with title, **due date** (relative + absolute, e.g. "tomorrow, 14:00"), and optionally priority, labels, project and assignees. Done tasks are not shown.
- W-VK-3: **Overdue tasks are clearly marked** (accent color, icon, "overdue by 2 days") and **sorted first**. Further visual states: due today, due soon, no due date. Default sort: overdue → due date ascending → no due date (priority as tie-breaker); sort is configurable.
- W-VK-4: Filtering per widget instance: project(s), label(s), saved Vikunja filter, assignee, due window (overdue / today / this week / all). A header line MAY additionally show summary counts (e.g. "3 overdue · 5 today").
- W-VK-5: **Mark task as done directly from the dashboard** with one tap on a large checkbox (respecting read-only mode, FR-31). Optimistic update with a short **undo** toast; on failure the item reappears and an error is shown. Recurring tasks MUST behave as in Vikunja (completing moves the due date; the list updates from the server response).
- W-VK-6: Long lists: the widget scrolls (touch) or paginates with a "+N more" indicator; the size variant decides how many rows are visible.
- W-VK-7: SHOULD allow quick-add of a task (e.g. shopping list with on-screen keyboard); detail view shows description, subtasks and all fields.
- W-VK-8: Live updates via polling (default ~30–60 s), optionally webhook. The "overdue" state is also re-evaluated locally by the frontend clock at the moment a task becomes due.

### 7.6 Grafana
- W-GF-1: **No iFrames:** the backend queries data and the frontend renders natively in the Lumina look.
- W-GF-2: **Grafana is the single data gateway** (decision Q6): Lumina does not implement individual datasource clients (Prometheus, InfluxDB, …). The backend uses the Grafana HTTP API with a **service-account token (Viewer role)** and runs queries through Grafana's datasource query endpoint (`/api/ds/query`), so every datasource Grafana supports is usable. A widget is configured with a datasource UID plus the query (copied from a Grafana panel), or, SHOULD, references an existing **dashboard panel** whose queries are read via the dashboard API and re-executed by Lumina. Query results (data frames) are mapped to Lumina's own chart/series model.
- W-GF-3: Native visualizations: **stat/single value** (with thresholds/colors), **sparkline**, **time-series line/area chart**, **gauge**, **bar**. Time range and refresh interval configurable.
- W-GF-4: Rendering server-side images (Grafana image renderer) is explicitly a fallback only and would be proxied as image, not embedded UI.
- W-GF-5: Must reuse the shared chart components from the design system so charts look consistent with all other widgets.

### 7.7 Paperless-ngx
- W-PL-1: Connect via REST API token.
- W-PL-2: Show: inbox/unprocessed document count, recently added documents (title, correspondent, date, tags), and counts per tag/correspondent/type.
- W-PL-3: Thumbnails (proxied through the backend, cached); detail view with metadata. Document opening/preview is SHOULD (native PDF/image rendering, no iFrame) or MAY be restricted to metadata only in v1.
- W-PL-4: Optional search; MAY highlight documents needing attention (e.g. tagged "to do").

### 7.8 Further candidates (to guide the abstraction, not in v1 scope)
Home Assistant, MQTT, Pi-hole/AdGuard, router/Fritz!Box status, public transport (departures), waste collection calendar, RSS/news, photo frame, energy/PV (e.g. Solar), Uptime Kuma, Docker/server status, notes, camera snapshots (image proxy), timer/alarm. The widget architecture MUST be validated against at least two of these on paper before it is finalised.

---

## 8. Configuration & Administration

- CF-1: Configuration lives in files in a config directory (YAML preferred, with JSON Schema for editor support): `connections`, `widgets`/`pages`, `theme`, `server`, `auth`.
- CF-2: Secrets can be referenced via env variables or secret files (e.g. `${VIKUNJA_TOKEN}` / `file:`), so the main config can be kept in a git repo.
- CF-3: Hot reload of config (file watch or admin action) pushes a new `config` message to all clients without a browser reload.
- CF-4: Admin UI (MAY, post-v1; YAML is the v1 way): manage connections, test connection button, edit layout, widget settings forms generated from the widget config schemas, view provider status/log.
- CF-5: Export/import of configuration (file copy is sufficient).
- CF-6: Per-device settings (e.g. which page is default, theme, ambient mode) SHOULD be possible, identified by a device name/token.

---

## 9. Technology Constraints and Decisions

| Area | Decision / Direction | Status |
|---|---|---|
| Backend language | Go (current stable) | decided |
| Transport | WebSocket (e.g. `nhooyr/coder websocket` or `gorilla/websocket`) | decided (library TBD) |
| Frontend | Modern framework, lightweight; candidates Svelte(Kit as SPA), Preact, SolidJS; no heavy meta-frameworks/SSR | TBD (ADR-001) |
| Build | Vite, output embedded into the Go binary (`go:embed`) | proposed |
| Styling | CSS custom properties (design tokens) + scoped CSS; no runtime CSS-in-JS | proposed |
| Charts | Own SVG/Canvas components or small lib (e.g. uPlot) – benchmark on target tablet | TBD (ADR-002) |
| Config | YAML + JSON Schema | proposed |
| Deployment | Container image on Kubernetes (single replica, ConfigMap/Secret, health probes) | decided |
| State | In-memory only (cache + ≤24 h history), no database/volume in v1 | decided |
| Embedding third-party UI | **Forbidden** (no iFrames) | decided |

Decisions are recorded as short ADRs in `docs/adr/`.

---

## 10. Phasing (proposal)

1. **M0 – Foundations:** repo layout, Go server skeleton, WS protocol, auth, config loading, frontend shell, design tokens, widget registry on both sides, Clock widget, CI.
2. **M1 – Framework hardening:** scheduler, cache, subscriptions, stale/error states, reconnect, page layout/grid, theming, kiosk features. Test on the real tablet.
3. **M2 – First integrations:** Weather, Calendar (iCal), Vikunja.
4. **M3 – More integrations:** openHAB (with actions), Paperless-ngx, Grafana, Google Calendar.
5. **M4 – Polish & admin:** edit mode / admin UI, ambient mode, PWA, metrics, docs, widget scaffold generator.

---

## 11. Open Questions

| # | Question | Impact |
|---|---|---|
| Q1 | ✅ *Answered: Android tablets, 4 GB RAM; exact model/WebView version unknown → test on device in M0.* Original: Which exact tablet(s)/OS/browser version(s) must be supported? (determines JS/CSS baseline, effects, framework choice) | Frontend tech, performance budget |
| Q2 | ✅ *Answered: secret iCal address for now.* Original: Google Calendar: is a "secret iCal address" acceptable, or is full OAuth/API access (private calendars, write) needed? | Calendar widget scope & complexity |
| Q3 | ✅ *Answered: Open-Meteo for v1 (provider abstraction kept).* Original: Weather: preferred provider / region (Germany → DWD/Open-Meteo?) and wanted data (warnings, radar?) | Weather widget |
| Q4 | ✅ *Answered: no login, LAN only.* Original: Authentication needs: LAN-only with no login, simple PIN/token, or SSO via reverse proxy? Any remote access wanted? | Security design |
| Q5 | ✅ *Answered: YAML is fine.* Original: Must layout be editable in the UI from day one, or is YAML config enough for v1? | Scope of M1/M4 |
| Q6 | ✅ *Answered: use Grafana as the single datasource gateway; no individual datasource clients.* Original: Grafana: which datasources are behind the panels (Prometheus, InfluxDB, …)? Is querying via Grafana's API acceptable? | Grafana widget design |
| Q7 | ✅ *Answered: German and English.* Original: UI languages (DE only, DE+EN)? | i18n effort |
| Q8 | ✅ *Answered: mainly display, with controls (openHAB switches, Vikunja complete).* Original: Should the dashboard be interactive (control lights, complete tasks) or primarily display-only? | Action/permission model |
| Q9 | ✅ *Answered: container image, hosted in own Kubernetes cluster; see A-5/A-6.* Original: Where does it run (Raspberry Pi, NAS, home server, Docker host)? Expected resources? | Deployment, binary targets |
| Q10 | ✅ *Answered: no persistence in v1; small in-memory history (e.g. last 24 h) is fine, cleared on restart; see FR-25a.* Original: Data retention: any need for history/trends stored by Lumina itself (e.g. sparkline of values), or always source-of-truth from the integrations? | Need for local DB (SQLite) |
| Q11 | ✅ *Answered: no tablet control.* Original: Should the tablet be controllable (screen on/off, brightness, e.g. via Fully Kiosk Browser API / openHAB)? | Ambient mode feature |

---

**Still open:** none for the initial scope. New questions will be added as the architecture decisions (framework, charts) and the tablet benchmark produce them.

---

## 12. Proposed Repository Structure (for orientation)

```
lumina/
├─ docs/
│  ├─ requirements.md        (this file)
│  ├─ adr/                   (architecture decision records)
│  └─ widgets/               (widget authoring guide, per-widget docs)
├─ backend/                  (Go module)
│  ├─ cmd/lumina/
│  └─ internal/{core,ws,scheduler,cache,config,auth,widgets/<type>}
├─ frontend/                 (SPA)
│  └─ src/{core,design-system,widgets/<type>}
├─ config/                   (example config, JSON Schema)
├─ deploy/                   (Dockerfile, Kubernetes manifests)
└─ README.md
```
