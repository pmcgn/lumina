<script lang="ts">
  import { scaleLinear } from 'd3-scale';
  import { area, curveMonotoneX, line } from 'd3-shape';

  let { points, color = 'var(--accent)' }: { points: { t: number; v: number }[]; color?: string } = $props();

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
</script>

<div class="spark" bind:clientWidth={width} bind:clientHeight={height}>
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
    </svg>
  {/if}
</div>

<style>
  .spark {
    width: 100%;
    height: 100%;
    min-height: 0;
  }
  svg {
    display: block;
  }
</style>
