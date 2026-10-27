<script lang="ts">
  import { i18n } from '../../lib/i18n.svelte';
  import type { WidgetProps } from '../../lib/registry';
  import AreaChart from '../../components/charts/AreaChart.svelte';
  import WeatherIcon from '../../components/WeatherIcon.svelte';
  import type { WeatherData, WeatherSettings } from './types';

  let { data, settings, size }: WidgetProps<WeatherData, WeatherSettings> = $props();

  const tz = $derived(data.timezone || undefined);
  const locale = $derived(i18n.locale);

  const deg = $derived(new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }));
  const temp = (v: number) => `${deg.format(v)}°`;

  // What fits depends on the card's pixel size, not on the screen.
  const compact = $derived(size.height < 190);
  const showChart = $derived(size.height >= 280);
  const showDaily = $derived(size.height >= 400);
  const dayCount = $derived(Math.max(3, Math.min(7, Math.floor(size.width / 92))));

  const points = $derived(data.hourly.map((h) => ({ t: h.t * 1000, v: h.tempC })));
  const bars = $derived(data.hourly.map((h) => ({ t: h.t * 1000, v: h.precipProb / 100 })));
  const hourFmt = $derived(new Intl.DateTimeFormat(locale, { hour: '2-digit', hour12: false, timeZone: tz }));
  const hourLabel = (t: number) => hourFmt.format(t); // the German format already includes "Uhr"

  const cur = $derived(data.current);
  const wind = $derived(`${deg.format(cur.windKmh)} km/h`);

  function weekday(date: string, index: number): string {
    if (index === 0) return i18n.t('weather.today');
    const [y, m, d] = date.split('-').map(Number);
    return new Intl.DateTimeFormat(locale, { weekday: 'short', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, d)));
  }

  const iconSize = $derived(compact ? 56 : Math.min(96, Math.max(64, size.height * 0.2)));
  const tempSize = $derived(compact ? 44 : Math.min(72, Math.max(48, size.height * 0.15)));
</script>

<div class="weather" class:compact>
  <header>
    <WeatherIcon condition={cur.condition} isDay={cur.isDay} size={iconSize} />
    <div class="now">
      <div class="temp" style:font-size="{tempSize}px">{temp(cur.tempC)}</div>
      <div class="cond">{i18n.t(`weather.condition.${cur.condition}`)}</div>
    </div>
    <div class="facts">
      {#if data.location}<div class="loc">{data.location}</div>{/if}
      {#if !compact}
        <div>{i18n.t('weather.feelsLike', { temp: temp(cur.feelsLikeC) })}</div>
        <div>{i18n.t('weather.humidity')} {cur.humidity} %</div>
        <div>{i18n.t('weather.wind')} {wind}</div>
      {/if}
    </div>
  </header>

  {#if showChart}
    <div class="chart">
      <AreaChart {points} {bars} formatX={hourLabel} formatY={temp} />
    </div>
  {/if}

  {#if showDaily}
    <ul class="days">
      {#each data.daily.slice(0, dayCount) as day, i (day.date)}
        <li>
          <span class="dow">{weekday(day.date, i)}</span>
          <WeatherIcon condition={day.condition} size={36} />
          <span class="hi">{temp(day.maxC)}</span>
          <span class="lo">{temp(day.minC)}</span>
          <span class="pp" class:dim={day.precipProb < 20}>{day.precipProb} %</span>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .weather {
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5);
  }
  .weather.compact {
    justify-content: center;
  }
  header {
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }
  .now {
    min-width: 0;
  }
  .temp {
    font-weight: 300;
    line-height: 1;
    letter-spacing: -0.03em;
  }
  .cond {
    margin-top: var(--space-1);
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .facts {
    margin-left: auto;
    text-align: right;
    color: var(--muted);
    font-size: var(--text-s);
    line-height: 1.5;
  }
  .loc {
    color: var(--text);
    font-size: var(--text-m);
    font-weight: 500;
  }
  .chart {
    flex: 1;
    min-height: 90px;
  }
  .days {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-3) 0 0;
    list-style: none;
    border-top: 1px solid var(--border);
  }
  .days li {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
  }
  .dow {
    color: var(--muted);
    font-size: var(--text-s);
  }
  .hi {
    font-weight: 600;
  }
  .lo {
    color: var(--muted);
    font-size: var(--text-s);
  }
  .pp {
    color: var(--accent-2);
    font-size: 0.75rem;
  }
  .pp.dim {
    opacity: 0.4;
  }
</style>
