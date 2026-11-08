<script lang="ts">
  import { scaleLinear } from 'd3-scale';
  import { area, curveMonotoneX, line } from 'd3-shape';
  import { bisector } from 'd3-array';
  import type { Inspected } from './TimeChart.svelte';

  interface Props {
    points: { t: number; v: number }[];
    color?: string;
    name?: string;
    /** While a finger / the mouse is down: the inspected time and value. The parent shows the readout. */
    inspected?: Inspected | null;
  }

  let { points, color = 'var(--accent)', name = '', inspected = $bindable(null) }: Props = $props();

  const uid = $props.id();
  let width = $state(0);
  let height = $state(0);

  const x = $derived(
    scaleLinear()
      .domain([points[0]?.t ?? 0, points[points.length - 1]?.t ?? 1])
      .range([0, width]),
  );
  const y = $derived.by(() => {
    let lo = Math.min(...points.map((p) => p.v));
    let hi = Math.max(...points.map((p) => p.v));
    if (!Number.isFinite(lo) || lo === hi) {
      lo = (Number.isFinite(lo) ? lo : 0) - 1;
      hi = lo + 2;
    }
    const pad = (hi - lo) * 0.12;
    return scaleLinear()
      .domain([lo - pad, hi + pad])
      .range([height, 0]);
  });
  const linePath = $derived(
    line<{ t: number; v: number }>().x((p) => x(p.t)).y((p) => y(p.v)).curve(curveMonotoneX)(points) ?? '',
  );
  const areaPath = $derived(
    area<{ t: number; v: number }>().x((p) => x(p.t)).y0(height).y1((p) => y(p.v)).curve(curveMonotoneX)(points) ?? '',
  );

  // Inspect with a finger or the mouse: the value at the nearest time.
  const nearest = bisector<{ t: number; v: number }, number>((p) => p.t).center;
  let active = $state<{ t: number; v: number } | null>(null);
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
    const t = x.invert(Math.min(Math.max(e.clientX - box.left, 0), width));
    active = points[nearest(points, t)] ?? null;
    inspected = active ? { t: active.t, rows: [{ name, color, v: active.v }] } : null;
  }
</script>

<div
  class="spark"
  bind:clientWidth={width}
  bind:clientHeight={height}
  role="presentation"
  onpointerdown={press}
  onpointermove={(e) => active && inspect(e)}
  onpointerup={release}
  onpointercancel={release}
  onlostpointercapture={release}
>
  {#if width > 0 && height > 0 && points.length > 1}
    <svg {width} {height} aria-hidden="true">
      <defs>
        <linearGradient id="spark-{uid}" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color={color} stop-opacity="0.35" />
          <stop offset="100%" stop-color={color} stop-opacity="0" />
        </linearGradient>
      </defs>
      <path d={areaPath} fill="url(#spark-{uid})" />
      <path d={linePath} fill="none" stroke={color} stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
      {#if active}
        <line class="cursor" x1={x(active.t)} x2={x(active.t)} y1="0" y2={height} />
        <circle cx={x(active.t)} cy={y(active.v)} r="4.5" fill={color} stroke="var(--surface)" stroke-width="2" />
      {/if}
    </svg>
  {/if}
</div>

<style>
  .spark {
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
  .cursor {
    stroke: var(--muted);
    stroke-dasharray: 3 3;
  }
</style>
