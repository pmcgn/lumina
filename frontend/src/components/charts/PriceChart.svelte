<script lang="ts">
  import { scaleLinear, scaleTime } from 'd3-scale';
  import { area, curveStepAfter, line } from 'd3-shape';
  import { bisector } from 'd3-array';
  import type { Inspected } from './TimeChart.svelte';

  export interface PricePoint {
    t: number; // start of the interval, unix milliseconds
    v: number;
  }

  interface Props {
    points: PricePoint[];
    stepMs: number;
    /** Current time (unix ms): marks the running interval. */
    now: number;
    formatX: (t: number) => string;
    formatY: (v: number) => string;
    /** Label of the marker at the current price (default: formatY). */
    formatNow?: (v: number) => string;
    color?: string;
    name?: string;
    /** While a finger / the mouse is down: the inspected interval. The parent shows the readout. */
    inspected?: Inspected | null;
  }

  let { points, stepMs, now, formatX, formatY, formatNow = formatY, color = 'var(--accent)', name = '', inspected = $bindable(null) }: Props = $props();

  const uid = $props.id();
  const top = 10;
  const bottom = 22;
  const right = 10;

  let width = $state(0);
  let height = $state(0);

  // a step needs an end: repeat the last price for one more interval
  const steps = $derived(points.length ? [...points, { t: points[points.length - 1].t + stepMs, v: points[points.length - 1].v }] : []);
  const from = $derived(points.length ? points[0].t : 0);
  const to = $derived(steps.length ? steps[steps.length - 1].t : 1);
  const domain = $derived.by((): [number, number] => {
    const vs = points.map((p) => p.v);
    const lo = Math.min(0, ...vs);
    const hi = Math.max(0, ...vs);
    return hi === lo ? [lo, lo + 1] : [lo, hi];
  });
  const y = $derived(scaleLinear().domain(domain).nice(4).range([Math.max(0, height - top - bottom), 0]));
  const yTicks = $derived(y.ticks(4));
  const left = $derived(10 + Math.max(0, ...yTicks.map((v) => formatY(v).length)) * 6.6);
  const innerW = $derived(Math.max(0, width - left - right));
  const innerH = $derived(Math.max(0, height - top - bottom));
  const x = $derived(scaleTime().domain([new Date(from), new Date(to)]).range([0, innerW]));
  const xTicks = $derived(innerW > 0 ? x.ticks(Math.max(2, Math.floor(innerW / 80))) : []);

  const linePath = $derived(
    line<PricePoint>()
      .x((p) => x(new Date(p.t)))
      .y((p) => y(p.v))
      .curve(curveStepAfter)(steps) ?? '',
  );
  const areaPath = $derived(
    area<PricePoint>()
      .x((p) => x(new Date(p.t)))
      .y0(y(0))
      .y1((p) => y(p.v))
      .curve(curveStepAfter)(steps) ?? '',
  );

  const midnights = $derived(
    points
      .filter((p, i) => i > 0 && new Date(p.t).getHours() === 0 && new Date(p.t).getMinutes() === 0)
      .map((p) => x(new Date(p.t))),
  );
  const nowPoint = $derived(points.find((p) => p.t <= now && now < p.t + stepMs));

  const nearest = bisector<PricePoint, number>((p) => p.t).center;
  let active = $state<PricePoint | null>(null);
  function release() {
    active = null;
    inspected = null;
  }
  function press(e: PointerEvent) {
    try {
      (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    } catch {
      /* not capturable: moves still arrive while over the chart */
    }
    inspect(e);
  }
  function inspect(e: PointerEvent) {
    const box = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const px = Math.min(Math.max(e.clientX - box.left - left, 0), innerW);
    // the interval under the finger
    const p = points[nearest(points, x.invert(px).getTime() - stepMs / 2)];
    if (!p) return release();
    active = p;
    inspected = { t: p.t, rows: [{ name, color, v: p.v }] };
  }
</script>

<div
  class="chart"
  bind:clientWidth={width}
  bind:clientHeight={height}
  role="presentation"
  data-no-swipe
  onpointerdown={press}
  onpointermove={(e) => active && inspect(e)}
  onpointerup={release}
  onpointercancel={release}
  onlostpointercapture={release}
>
  {#if width > 0 && height > 0 && points.length > 1}
    <svg {width} {height} aria-hidden="true">
      <defs>
        <linearGradient id="fill-{uid}" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color={color} stop-opacity="0.35" />
          <stop offset="100%" stop-color={color} stop-opacity="0" />
        </linearGradient>
      </defs>
      <g transform="translate({left},{top})">
        {#each yTicks as v (v)}
          <line class="grid" x1="0" x2={innerW} y1={y(v)} y2={y(v)} />
          <text class="axis" x="-8" y={y(v) + 4} text-anchor="end">{formatY(v)}</text>
        {/each}
        {#each xTicks as t (t.getTime())}
          <text class="axis" x={x(t)} y={innerH + 16} text-anchor="middle">{formatX(t.getTime())}</text>
        {/each}

        <path d={areaPath} fill="url(#fill-{uid})" />
        <path d={linePath} fill="none" stroke={color} stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />

        {#each midnights as mx (mx)}
          <line class="midnight" x1={mx} x2={mx} y1="0" y2={innerH} />
        {/each}
        {#if nowPoint && !active}
          {@const nx = x(new Date(now))}
          {@const ny = y(nowPoint.v)}
          <line class="now" x1={nx} x2={nx} y1={ny} y2={y(0)} />
          <circle cx={nx} cy={ny} r="4.5" fill={color} stroke="var(--surface)" stroke-width="2" />
          <text
            class="value"
            x={Math.min(Math.max(nx, 24), innerW - 24)}
            y={ny < 24 ? ny + 20 : ny - 10}
            text-anchor="middle">{formatNow(nowPoint.v)}</text
          >
        {/if}

        {#if active}
          {@const ax = x(new Date(active.t + stepMs / 2))}
          <line class="cursor" x1={ax} x2={ax} y1="0" y2={innerH} />
          <circle cx={ax} cy={y(active.v)} r="4.5" fill={color} stroke="var(--surface)" stroke-width="2" />
        {/if}
      </g>
    </svg>
  {/if}
</div>

<style>
  .chart {
    position: relative;
    width: 100%;
    height: 100%;
    min-height: 0;
    touch-action: none; /* a finger on the chart inspects it instead of scrolling */
    user-select: none;
    -webkit-user-select: none;
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
  .now {
    stroke: var(--accent);
    stroke-opacity: 0.5;
    stroke-width: 1;
    stroke-dasharray: 3 3;
  }
  .value {
    fill: var(--text);
    font-size: 12px;
    font-weight: 600;
    paint-order: stroke; /* a halo keeps the label readable over the curve */
    stroke: var(--surface);
    stroke-width: 3px;
    stroke-linejoin: round;
  }
  .midnight {
    stroke: var(--border);
    stroke-width: 1;
  }
  .cursor {
    stroke: var(--muted);
    stroke-dasharray: 3 3;
  }
</style>
