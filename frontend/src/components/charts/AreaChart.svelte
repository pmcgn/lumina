<script lang="ts">
  import { scaleLinear, scaleTime } from 'd3-scale';
  import { area, curveMonotoneX, line } from 'd3-shape';
  import { bisector, extent } from 'd3-array';

  export interface ChartPoint {
    t: number; // unix milliseconds
    v: number;
  }

  interface Props {
    points: ChartPoint[];
    /** Optional bars drawn behind the curve; v is a fraction 0..1 of the bar height. */
    bars?: ChartPoint[];
    formatX: (t: number) => string;
    formatY: (v: number) => string;
    color?: string;
  }

  let { points, bars = [], formatX, formatY, color = 'var(--accent)' }: Props = $props();

  const uid = $props.id();
  const m = { top: 22, right: 14, bottom: 24, left: 14 };

  let width = $state(0);
  let height = $state(0);

  const innerW = $derived(Math.max(0, width - m.left - m.right));
  const innerH = $derived(Math.max(0, height - m.top - m.bottom));

  const x = $derived(
    scaleTime()
      .domain(extent(points, (p) => new Date(p.t)) as [Date, Date])
      .range([0, innerW]),
  );
  const y = $derived.by(() => {
    const [lo, hi] = extent(points, (p) => p.v) as [number, number];
    const pad = Math.max((hi - lo) * 0.18, 0.5);
    return scaleLinear()
      .domain([lo - pad, hi + pad])
      .range([innerH, 0]);
  });

  const linePath = $derived(
    line<ChartPoint>()
      .x((p) => x(new Date(p.t)))
      .y((p) => y(p.v))
      .curve(curveMonotoneX)(points) ?? '',
  );
  const areaPath = $derived(
    area<ChartPoint>()
      .x((p) => x(new Date(p.t)))
      .y0(innerH)
      .y1((p) => y(p.v))
      .curve(curveMonotoneX)(points) ?? '',
  );

  const ticks = $derived(
    innerW > 0 ? x.ticks(Math.max(2, Math.floor(innerW / 70))).map((d) => ({ x: x(d), label: formatX(d.getTime()) })) : [],
  );

  // Extremes get a value label.
  const hi = $derived(points.reduce((a, p) => (p.v > a.v ? p : a), points[0]));
  const lo = $derived(points.reduce((a, p) => (p.v < a.v ? p : a), points[0]));

  const barW = $derived(points.length > 1 ? (innerW / (points.length - 1)) * 0.62 : 0);

  // Touch/mouse inspection: nearest point under the finger.
  const nearest = bisector<ChartPoint, number>((p) => p.t).center;
  let active = $state<ChartPoint | null>(null);
  function inspect(e: PointerEvent) {
    const box = (e.currentTarget as SVGElement).getBoundingClientRect();
    const t = x.invert(e.clientX - box.left - m.left).getTime();
    active = points[nearest(points, t)] ?? null;
  }
</script>

<div class="chart" bind:clientWidth={width} bind:clientHeight={height}>
  {#if width > 0 && height > 0 && points.length > 1}
    <svg
      {width}
      {height}
      role="img"
      onpointerdown={inspect}
      onpointermove={(e) => active && inspect(e)}
      onpointerup={() => (active = null)}
      onpointercancel={() => (active = null)}
      onpointerleave={() => (active = null)}
    >
      <defs>
        <linearGradient id="fill-{uid}" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color={color} stop-opacity="0.38" />
          <stop offset="100%" stop-color={color} stop-opacity="0" />
        </linearGradient>
      </defs>
      <g transform="translate({m.left},{m.top})">
        {#each bars as b (b.t)}
          <rect
            class="bar"
            x={x(new Date(b.t)) - barW / 2}
            y={innerH - b.v * innerH * 0.38}
            width={barW}
            height={b.v * innerH * 0.38}
            rx="2"
          />
        {/each}

        <path d={areaPath} fill="url(#fill-{uid})" />
        <path d={linePath} fill="none" stroke={color} stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />

        {#each ticks as tick (tick.x)}
          <text class="axis" x={tick.x} y={innerH + 17} text-anchor="middle">{tick.label}</text>
        {/each}

        {#each [hi, lo] as p, i (i)}
          {#if hi.v !== lo.v}
            <circle cx={x(new Date(p.t))} cy={y(p.v)} r="3.5" fill={color} />
            <text
              class="value"
              x={Math.min(Math.max(x(new Date(p.t)), 14), innerW - 14)}
              y={y(p.v) + (i === 0 ? -9 : 17)}
              text-anchor="middle">{formatY(p.v)}</text
            >
          {/if}
        {/each}

        {#if active}
          <line class="cursor" x1={x(new Date(active.t))} x2={x(new Date(active.t))} y1="-6" y2={innerH} />
          <circle cx={x(new Date(active.t))} cy={y(active.v)} r="5" fill={color} stroke="var(--surface)" stroke-width="2" />
          <text
            class="tip"
            x={Math.min(Math.max(x(new Date(active.t)), 44), innerW - 44)}
            y="-8"
            text-anchor="middle">{formatX(active.t)} · {formatY(active.v)}</text
          >
        {/if}
      </g>
    </svg>
  {/if}
</div>

<style>
  .chart {
    width: 100%;
    height: 100%;
    min-height: 0;
    touch-action: pan-y;
  }
  svg {
    display: block;
    overflow: visible;
  }
  .axis {
    fill: var(--muted);
    font-size: 11px;
  }
  .value {
    fill: var(--text);
    font-size: 12px;
    font-weight: 600;
  }
  .tip {
    fill: var(--text);
    font-size: 12px;
    font-weight: 600;
  }
  .bar {
    fill: var(--accent-2);
    opacity: 0.45;
  }
  .cursor {
    stroke: var(--muted);
    stroke-width: 1;
    stroke-dasharray: 3 3;
  }
</style>
