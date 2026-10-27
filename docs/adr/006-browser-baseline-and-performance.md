# ADR-006: Browser baseline and performance strategy without target hardware

Status: accepted · Date: 2026-10-17

## Context
The target is an older Android tablet with 4 GB RAM. The exact model and WebView version are unknown and no such device is available for testing. We cannot validate on real hardware early, so we reduce risk by design and by emulation.

## Decision
1. **Working assumption:** Android System WebView / Chrome **≥ 80**. Vite `build.target` is `chrome80` (esbuild transpiles newer syntax). CSS avoids features newer than that (checked via `browserslist` and Autoprefixer): no `:has()`, no container queries, and flex `gap` / `inset` only with care. A lower baseline can be set later by changing the target, at the cost of a slightly bigger bundle.
2. **Container-size-based widget layout** with CSS Grid and `ResizeObserver` (Chrome 64+) instead of container queries.
3. **Performance budget**, enforced in CI where possible: initial JS ≤ 150 KB gzip, no single widget chunk > 50 KB gzip (excluding charts), animation guidelines from NFR-4/5.
4. **"Lite mode" switch** (config and per device) that disables transitions and large gradients and reduces update rates.
5. **Memory hygiene by construction:** no unbounded arrays in the frontend (series are capped), listeners and timers cleaned up on component teardown, and a **scheduled soft reload** (nightly by default, configurable) as a safety net (NFR-7).
6. **Emulation instead of hardware:** test with Chrome DevTools CPU throttling (4–6×) and, optionally, an Android Studio emulator image with an older API level and WebView (e.g. API 28–29, 2–4 GB RAM) to check rendering, touch and memory behaviour. A small performance test page (many widgets updating every second, plus a 24 h chart) is part of M0 so regressions show up early.
7. **Escape hatches** if issues appear on the real device: switch charts to canvas (ADR-002), enable lite mode, or swap the framework to Preact (ADR-001).

## Alternatives considered
- A dedicated device benchmark before starting: not possible (no hardware). Replaced by points 3–6.
- Targeting only modern browsers: risks an unusable dashboard on the actual tablet.

## Consequences
- Slight bundle overhead from transpilation; accepted.
- Real-device behaviour remains a known residual risk until the first test on the actual tablet. Points 1 and 4 make it cheap to react.
