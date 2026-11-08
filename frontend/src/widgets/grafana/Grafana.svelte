<script lang="ts">
  import { i18n } from '../../lib/i18n.svelte';
  import type { WidgetProps } from '../../lib/registry';
  import { formatParts, formatValue } from '../../lib/units';
  import Sparkline from '../../components/charts/Sparkline.svelte';
  import TimeChart, { type Inspected } from '../../components/charts/TimeChart.svelte';
  import ChartReadout from '../../components/charts/ChartReadout.svelte';
  import BatteryIcon from'../../components/BatteryIcon.svelte';
  import { arcPath, batteryColor, batteryPercent, fraction, gaugeRange, hasThresholds, last, seriesColor, thresholdColor, timeFormat } from './logic';
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

  // finger readout above the chart: shown only while a finger is down
  let inspected = $state<Inspected | null>(null);
  const readoutFormat = $derived(
    new Intl.DateTimeFormat(locale, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false }),
  );
  const readoutTime = (t: number) => readoutFormat.format(t);

  // battery: 0..100 whatever the panel's min/max say
  const battStyle = $derived(settings.batteryStyle ?? 'classic');
  const orientation = $derived(settings.batteryOrientation ?? 'horizontal');
  const valueMode = $derived(settings.batteryValue ?? 'side');
  const pct = $derived(batteryPercent(value, data.unit));
  const battColor = $derived(batteryColor(pct, data.thresholds));
  const battUnit = $derived(parts.unit || '%');
  // long side of the icon in px: as big as the card allows, leaving room for the heading, the name and the side value
  const battLength = $derived.by(() => {
    const aw = Math.max(0, size.width - 44);
    const ah = Math.max(0, size.height - (first?.name ? 96 : 72));
    const byWidth = valueMode === 'side' ? aw * 0.46 : aw * 0.9;
    const max = orientation === 'vertical' ? Math.min(ah, byWidth / (60 / 104)) : Math.min(byWidth, ah * (104 / 56));
    return Math.round(Math.min(280, Math.max(56, max)));
  });
</script>

<div class="g">
  {#if inspected}
    <ChartReadout {inspected} formatT={readoutTime} formatV={fmt} />
  {/if}
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
      <TimeChart {series} from={data.from} to={data.to} formatX={xFormat} formatY={fmt} min={data.min} max={data.max} bind:inspected />
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
      {#if first?.name}<div class="name">{first.name}</div>{/if}
      {#if first && first.points.length > 1 && !compact}
        <div class="spark"><Sparkline points={first.points} name={first.name} color={hasThresholds(data.thresholds) ? accent : first.color} bind:inspected /></div>
      {/if}
    </div>
  {:else if data.display === 'battery'}
    <div class="battery">
      <div class="row">
        <BatteryIcon
          percent={pct}
          {orientation}
          variant={battStyle}
          color={battColor}
          length={battLength}
          text={valueMode === 'inline' ? parts.value + battUnit : undefined}
        />
        {#if valueMode === 'side'}
          <div class="big" style:color={battStyle === 'classic' && hasThresholds(data.thresholds) ? battColor : 'var(--text)'}>
            <span class="num">{parts.value}</span><span class="unit">{battUnit}</span>
          </div>
        {/if}
      </div>
      {#if first?.name}<div class="name">{first.name}</div>{/if}
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
      {#if first?.name}<div class="name">{first.name}</div>{/if}
    </div>
  {/if}
</div>

<style>
  .g {
    position: relative;
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

  .battery {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
  }
  .battery .big {
    font-size: clamp(2rem, 5.5vw, 3.6rem);
  }
  .battery .row {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-4);
    max-width: 100%;
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
