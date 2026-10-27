<script lang="ts">
  import { scaleLinear, scaleTime } from 'd3-scale';
  import { area, curveMonotoneX, line } from 'd3-shape';
  import { bisector } from 'd3-array';

  export interface ChartSeries {
    name: string;
    color: string;
    points: { t: number; v: number }[]; // t in unix ms, ascending
  }

  interface Props {
    series: ChartSeries[];
    from: number; // unix ms, left edge
    to: number; // unix ms, right edge
    formatX: (t: number) => string;
    formatY: (v: number) => string;
    /** Fixed value range; default: from the data. */
    min?: number;
    max?: number;
  }

  let { series, from, to, formatX, formatY, min, max }: Props = $props();

  const uid = $props.id();
  let width = $state(0);
  let height = $state(0);
  const top = 10;
  const bottom = 22;
  const right = 10;

  const all = $derived(series.flatMap((s) => s.points));
  const domain = $derived.by((): [number, number] => {
    let lo = min ?? Math.min(...all.map((p) => p.v));
    let hi = max ?? Math.max(...all.map((p) => p.v));
    if (!Number.isFinite(lo) || !Number.isFinite(hi)) return [0, 1];
    if (lo === hi) {
      lo -= 1;
      hi += 1;
    }
    return [lo, hi];
  });
  const y = $derived(scaleLinear().domain(domain).nice(4).range([Math.max(0, height - top - bottom), 0]));
  const yTicks = $derived(y.ticks(4));
  const left = $derived(10 + Math.max(0, ...yTicks.map((v) => formatY(v).length)) * 6.6);
  const innerW = $derived(Math.max(0, width - left - right));
  const innerH = $derived(Math.max(0, height - top - bottom));
  const x = $derived(scaleTime().domain([new Date(from), new Date(to)]).range([0, innerW]));
  const xTicks = $derived(innerW > 0 ? x.ticks(Math.max(2, Math.floor(innerW / 80))) : []);

  // Data gaps (much larger than the usual spacing) break the line.
  function segments(pts: { t: number; v: number }[]) {
    if (pts.length < 2) return [pts];
    const gaps = pts.slice(1).map((p, i) => p.t - pts[i].t).sort((a, b) => a - b);
    const limit = gaps[Math.floor(gaps.length / 2)] * 6;
    const out: (typeof pts)[] = [[pts[0]]];
    for (let i = 1; i < pts.length; i++) {
      if (pts[i].t - pts[i - 1].t > limit) out.push([]);
      out[out.length - 1].push(pts[i]);
    }
    return out;
  }

  const lineGen = $derived(
    line<{ t: number; v: number }>()
      .x((p) => x(new Date(p.t)))
      .y((p) => y(p.v))
      .curve(curveMonotoneX),
  );
  const areaGen = $derived(
    area<{ t: number; v: number }>()
      .x((p) => x(new Date(p.t)))
      .y0(innerH)
      .y1((p) => y(p.v))
      .curve(curveMonotoneX),
  );
  const paths = $derived(
    series.map((s) => ({
      color: s.color,
      lines: segments(s.points).map((seg) => lineGen(seg) ?? ''),
      areas: series.length === 1 ? segments(s.points).map((seg) => areaGen(seg) ?? '') : [],
    })),
  );

  // Inspect with a finger or the mouse: values of all series at the nearest time.
  const nearest = bisector<{ t: number; v: number }, number>((p) => p.t).center;
  let cursor = $state<{ px: number; t: number; rows: { name: string; color: string; v: number }[] } | null>(null);
  function inspect(e: PointerEvent) {
    const box = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const px = Math.min(Math.max(e.clientX - box.left - left, 0), innerW);
    const t = x.invert(px).getTime();
    const rows = series
      .filter((s) => s.points.length)
      .map((s) => ({ name: s.name, color: s.color, v: s.points[nearest(s.points, t)].v }));
    const first = series.find((s) => s.points.length);
    cursor = { px, t: first ? first.points[nearest(first.points, t)].t : t, rows };
  }
</script>

<div
  class="chart"
  bind:clientWidth={width}
  bind:clientHeight={height}
  role="presentation"
  onpointerdown={inspect}
  onpointermove={(e) => cursor && inspect(e)}
  onpointerup={() => (cursor = null)}
  onpointercancel={() => (cursor = null)}
  onpointerleave={() => (cursor = null)}
>
  {#if width > 0 && height > 0 && all.length > 1}
    <svg {width} {height} aria-hidden="true">
      <defs>
        {#each series as s, i (i)}
          <linearGradient id="fill-{uid}-{i}" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color={s.color} stop-opacity="0.35" />
            <stop offset="100%" stop-color={s.color} stop-opacity="0" />
          </linearGradient>
        {/each}
      </defs>
      <g transform="translate({left},{top})">
        {#each yTicks as v (v)}
          <line class="grid" x1="0" x2={innerW} y1={y(v)} y2={y(v)} />
          <text class="axis" x="-8" y={y(v) + 4} text-anchor="end">{formatY(v)}</text>
        {/each}
        {#each xTicks as t (t.getTime())}
          <text class="axis" x={x(t)} y={innerH + 16} text-anchor="middle">{formatX(t.getTime())}</text>
        {/each}

        {#each paths as p, i (i)}
          {#each p.areas as d, k (k)}<path {d} fill="url(#fill-{uid}-{i})" />{/each}
          {#each p.lines as d, k (k)}
            <path {d} fill="none" stroke={p.color} stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
          {/each}
        {/each}

        {#if cursor}
          <line class="cursor" x1={cursor.px} x2={cursor.px} y1="0" y2={innerH} />
          {#each series as s, i (i)}
            {#if s.points.length}
              {@const pt = s.points[nearest(s.points, cursor.t)]}
              <circle cx={x(new Date(pt.t))} cy={y(pt.v)} r="4.5" fill={s.color} stroke="var(--surface)" stroke-width="2" />
            {/if}
          {/each}
        {/if}
      </g>
    </svg>
    {#if cursor}
      <div class="tip" style:left="{Math.min(Math.max(left + cursor.px, 90), width - 90)}px">
        <div class="when">{formatX(cursor.t)}</div>
        {#each cursor.rows as r (r.name)}
          <div class="row"><i style:background={r.color}></i><span class="n">{r.name}</span><b>{formatY(r.v)}</b></div>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .chart {
    position: relative;
    width: 100%;
    height: 100%;
    min-height: 0;
    touch-action: pan-y;
  }
  svg {
    display: block;
    overflow: visible;
  }
  .grid {
    stroke: var(--border);
    stroke-width: 1;
  }
  .axis {
    fill: var(--muted);
    font-size: 11px;
  }
  .cursor {
    stroke: var(--muted);
    stroke-dasharray: 3 3;
  }
  .tip {
    position: absolute;
    top: 0;
    transform: translateX(-50%);
    min-width: 120px;
    max-width: 220px;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-s);
    background: var(--surface-2);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
    font-size: 12px;
    pointer-events: none;
  }
  .when {
    color: var(--muted);
    margin-bottom: 2px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .row i {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }
  .row .n {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
