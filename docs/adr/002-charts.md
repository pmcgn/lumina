# ADR-002: Charts – own SVG components built on d3 primitives

Status: accepted · Date: 2026-10-17

## Context
Charts must be beautiful and consistent with the design system (W-GF-5), render fast on a weak tablet, and show 24 h of data (FR-25a) from Grafana, openHAB or weather. Typical load: a few hundred points per series after downsampling, several charts on screen, updates every few seconds to minutes. Needed types: sparkline, line/area, bar, gauge, stat with thresholds.

## Decision
Implement a small set of **Svelte chart components rendering SVG**, using only the modular, tree-shakeable **`d3-scale`, `d3-shape` and `d3-array`** for the maths. No full chart library.

- Data is **downsampled on the backend** to roughly the pixel width of the chart (≤ ~300–500 points, e.g. LTTB or bucket averaging), so the frontend never draws more points than are visible.
- Look: smooth monotone curves, gradient area fills, rounded bars, thin axes, colors from CSS design tokens, subtle enter transitions.
- No SVG filters, blur or large drop shadows (expensive on weak GPUs); gradients and opacity only.
- Sparklines and gauges are separate, even lighter components (a single `<path>` / `<circle>`).
- Touch interaction: tap/drag shows a value crosshair and tooltip (no hover dependence).
- Palette comes from design tokens and is checked for contrast in light and dark mode.

## Alternatives considered
- **uPlot (canvas):** fastest and tiny, ideal for very dense data. Styling is limited, though, and our downsampled volumes do not need its speed. It is the **fallback** behind the same chart-component interface if SVG proves too slow.
- **Chart.js / Apache ECharts:** feature-rich but heavy (100 KB to 1 MB), and the canvas look differs from the rest of the UI.
- **Observable Plot / LayerCake:** nice, but more dependencies than we need.

## Consequences
- We own the chart code (more work than a library, but full control over look and size).
- Chart components take a normalised series model (`{t, v}[]`, optional thresholds and unit), so the renderer can be swapped (SVG ↔ canvas) without touching widgets.
- The backend must provide a downsampling helper for the history store and the Grafana provider.
