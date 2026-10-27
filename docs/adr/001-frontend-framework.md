# ADR-001: Frontend framework – Svelte 5 SPA (no SvelteKit)

Status: accepted · Date: 2026-10-17

## Context
Requirements: runs smoothly on older Android tablets (4 GB RAM, WebView version unknown), runs 24/7 without memory growth (NFR-7), small bundle (NFR-3), fine-grained updates of single values (NFR-6), many independent widgets with scoped styling, German/English i18n, touch UI with smooth animations. The backend is Go and serves static assets; no SSR is needed.

## Decision
Use **Svelte 5** (runes) built with **Vite** as a plain single-page app (no SvelteKit, no SSR), written in **TypeScript**.

- Components are compiled to direct DOM updates; there is no virtual DOM and the runtime is tiny, which lowers CPU/memory use on weak devices.
- Fine-grained reactivity (`$state`, `$derived`) means a WebSocket update to one value touches only the DOM nodes that use it.
- Scoped CSS per component fits the model "each widget brings its own styles, shared tokens via CSS variables" (WA-10).
- Built-in transitions, stores and context cover our needs without extra libraries.
- Routing is trivial (pages are state, not URLs), so no router library.
- i18n via a small hand-written store with JSON message files per language (DE, EN).

## Alternatives considered
- **Preact (+ signals):** smallest runtime and best legacy compatibility, large ecosystem. But vdom diffing costs more per update, and scoped styling/animation need extra libraries. Strong runner-up and the fallback if Svelte 5 output proves problematic on the baseline browser.
- **SolidJS:** comparable fine-grained performance, but a smaller ecosystem and fewer built-ins (transitions, styling).
- **React / Vue / Angular:** larger runtime and bundle, no advantage for this app.
- **Web components / Lit:** viable and light, but weaker developer experience for reactive state shared across many widgets.

## Consequences
- Svelte 5 relies on modern JS (e.g. Proxy); the build target is set in ADR-006. If the real tablet turns out to be too old, we fall back to Preact. Widgets are thin (props in, markup out), so a port is bounded.
- Widget authors need Svelte knowledge (acceptable for a single maintainer).
- Shared design-system components are written once in Svelte and reused by all widgets.
