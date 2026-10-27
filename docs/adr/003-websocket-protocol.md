# ADR-003: WebSocket protocol and Go library

Status: accepted · Date: 2026-10-17

## Context
All frontend ⇄ backend communication runs over one WebSocket (A-3). Needs: per-widget subscriptions, snapshot on (re)connect, updates, actions (toggle light, complete task) with results, heartbeats, version handshake, multiple clients, LAN-only without auth.

## Decision
- **Library (Go):** `github.com/coder/websocket` (maintained, context-aware, concurrency-safe writes, built-in ping, origin checking). `gorilla/websocket` remains an acceptable alternative.
- **Encoding:** JSON text frames in a versioned envelope:
  ```json
  { "v": 1, "type": "update", "id": "optional-correlation-id", "widget": "tasks-main", "ts": 1760000000000, "payload": {} }
  ```
- **Client → server:** `hello` (client id, locale, app version, device name), `subscribe` / `unsubscribe` (widget ids visible on the current page), `action` (widget, name, params, correlation id), `pong`.
- **Server → client:** `welcome` (server version; the client reloads on mismatch), `config` (pages, layout, widget definitions, theme; re-sent on hot reload), `snapshot` (full widget data plus status ok / stale / error), `update` (v1: full widget data replacement; JSON patch optional later), `action_result`, `error`, `ping`.
- **Delivery model:** the core fetches a widget instance's data once and fans it out to all subscribed clients. On subscribe the client immediately gets the cached snapshot. Updates are sent only if the data changed (hash compare) and are coalesced (max one update per widget per ~250 ms).
- **Reconnect:** exponential backoff with jitter. On `welcome` the client resubscribes and receives fresh snapshots; no replay or sequence numbers are needed.
- **Heartbeat:** server ping every ~15 s; connection dropped after ~30 s without a reply. The client also detects silence.
- **Security:** same-origin check on upgrade, frame size limit, actions validated against the widget's read-only flag and the provider's declared actions.
- **Schema:** message and widget-data types are defined in Go and exported as JSON Schema, then generated into TypeScript types (WA-11).

## Alternatives considered
- MessagePack/CBOR: smaller, but JSON is easier to debug and payloads are small. Revisit if needed.
- Per-widget patches from day one: more complexity for little gain at our data sizes.
- SSE + REST: contradicts the WebSocket-only decision and complicates actions.

## Consequences
- Simple protocol that is easy to test (record and replay of messages).
- Large series (charts) are sent as full snapshots. This is acceptable because they are downsampled (ADR-002) and change slowly.
