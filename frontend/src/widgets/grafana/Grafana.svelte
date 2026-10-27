<script lang="ts">
  import { i18n } from '../../lib/i18n.svelte';
  import type { WidgetProps } from '../../lib/registry';
  import { formatParts, formatValue } from '../../lib/units';
  import Sparkline from '../../components/charts/Sparkline.svelte';
  import TimeChart from '../../components/charts/TimeChart.svelte';
  import { arcPath, fraction, gaugeRange, hasThresholds, last, seriesColor, thresholdColor, timeFormat } from './logic';
  import type { Data, Settings } from './model.gen';

  let { data, settings, size }: WidgetProps<Data, Settings> = $props();
  const t = i18n.t;
  const locale = $derived(i18n.locale);

  const series = $derived(data.series.map((s, i) => ({ ...s, color: seriesColor(s.color, i) })));
  const hasData = $derived(series.some((s) => s.points.length > 0));
  const fmt = (v: number) => formatValue(v, data.unit, data.decimals, locale);

  // stat and gauge show the latest value of the first series
  const first = $derived(series.find((s) => s.points.length > 0));
  const value = $derived(first ? last(first.points)!.v : NaN);
  const parts = $derived(formatParts(value, data.unit, data.decimals, locale));
  const accent = $derived(thresholdColor(value, data.thresholds, 'var(--accent)'));
  const valueColor = $derived(hasThresholds(data.thresholds) ? accent : 'var(--text)');

  const compact = $derived(size.height < 170);
  const showLegend = $derived(settings.legend ?? series.length > 1);

  // gauge geometry
  const range = $derived(gaugeRange(data, value));
  const frac = $derived(fraction(value, range[0], range[1]));
  const R = 80;
  const track = arcPath(100, 100, R, 0, 1);
  const valueArc = $derived(arcPath(100, 100, R, 0, Math.max(frac, 0.004)));
  const xFormat = $derived(timeFormat(data.to - data.from, locale));
</script>

<div class="g">
  <header>
    <h2>{data.title}</h2>
    {#if data.warnings?.length}
      <span class="warn" title={data.warnings.join('\n')}>⚠ {t('grafana.partial')}</span>
    {/if}
  </header>

  {#if !hasData}
    <div class="empty">{t('grafana.noData')}</div>
  {:else if data.display === 'chart'}
    <div class="plot">
      <TimeChart {series} from={data.from} to={data.to} formatX={xFormat} formatY={fmt} min={data.min} max={data.max} />
    </div>
    {#if showLegend && !compact}
      <ul class="legend">
        {#each series as s (s.name)}
          <li>
            <i style:background={s.color}></i>
            <span class="n">{s.name}</span>
            {#if last(s.points)}<b>{fmt(last(s.points)!.v)}</b>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  {:else if data.display === 'stat'}
    <div class="stat">
      <div class="big" style:color={valueColor}>
        <span class="num">{parts.value}</span>{#if parts.unit}<span class="unit">{parts.unit}</span>{/if}
      </div>
      {#if first && series.length > 0}<div class="name">{first.name}</div>{/if}
      {#if first && first.points.length > 1 && !compact}
        <div class="spark"><Sparkline points={first.points} color={hasThresholds(data.thresholds) ? accent : first.color} /></div>
      {/if}
    </div>
  {:else}
    <div class="gauge">
      <svg viewBox="0 0 200 118" aria-hidden="true">
        <path d={track} class="track" />
        <path d={valueArc} class="fill" style:stroke={accent} />
        <text x="100" y="92" class="gnum" text-anchor="middle" style:fill={valueColor}>{parts.value}{parts.unit ? ' ' + parts.unit : ''}</text>
        <text x="20" y="116" class="gend" text-anchor="middle">{formatValue(range[0], data.unit, data.decimals, locale)}</text>
        <text x="180" y="116" class="gend" text-anchor="middle">{formatValue(range[1], data.unit, data.decimals, locale)}</text>
      </svg>
      {#if first}<div class="name">{first.name}</div>{/if}
    </div>
  {/if}
</div>

<style>
  .g {
    height: 100%;
    display: flex;
    flex-direction: column;
    padding: var(--space-4) var(--space-4) var(--space-3) var(--space-5);
  }
  header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-2);
  }
  h2 {
    margin: 0;
    font-size: var(--text-m);
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .warn {
    flex: none;
    color: var(--warn);
    font-size: var(--text-s);
  }
  .empty {
    flex: 1;
    display: grid;
    place-items: center;
    color: var(--muted);
    text-align: center;
  }

  .plot {
    flex: 1;
    min-height: 0;
  }
  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-4);
    margin: var(--space-2) 0 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-s);
  }
  .legend li {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  .legend i {
    flex: none;
    width: 9px;
    height: 9px;
    border-radius: 50%;
  }
  .legend .n {
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 14em;
  }

  .stat {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-1);
  }
  .big {
    line-height: 1;
    font-weight: 300;
    letter-spacing: -0.02em;
    font-size: clamp(2.2rem, 7vw, 4.2rem);
  }
  .big .unit {
    margin-left: 0.25em;
    font-size: 0.42em;
    color: var(--muted);
    font-weight: 400;
    letter-spacing: 0;
  }
  .name {
    color: var(--muted);
    font-size: var(--text-s);
    text-align: center;
  }
  .spark {
    width: 100%;
    flex: 0 0 38%;
    min-height: 0;
    margin-top: var(--space-2);
  }

  .gauge {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
  }
  .gauge svg {
    width: 100%;
    max-height: 100%;
    max-width: 340px;
  }
  .track {
    fill: none;
    stroke: var(--border);
    stroke-width: 16;
    stroke-linecap: round;
  }
  .fill {
    fill: none;
    stroke-width: 16;
    stroke-linecap: round;
  }
  .gnum {
    font-size: 28px;
    font-weight: 300;
  }
  .gend {
    fill: var(--muted);
    font-size: 10px;
  }
</style>
