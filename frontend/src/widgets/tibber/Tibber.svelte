<script lang="ts">
  import { onMount } from 'svelte';
  import { i18n } from '../../lib/i18n.svelte';
  import type { WidgetProps } from '../../lib/registry';
  import PriceChart from '../../components/charts/PriceChart.svelte';
  import ChartReadout from '../../components/charts/ChartReadout.svelte';
  import type { Inspected } from '../../components/charts/TimeChart.svelte';
  import { currentSlot, levelOf, priceUnit, stats, thresholds } from './prices';
  import type { TibberData, TibberSettings } from './types';

  let { data, size }: WidgetProps<TibberData, TibberSettings> = $props();

  const locale = $derived(i18n.locale);
  const tz = $derived(data.timezone || undefined);

  let now = $state(Date.now());
  onMount(() => {
    const timer = window.setInterval(() => (now = Date.now()), 30_000);
    return () => window.clearInterval(timer);
  });

  const unit = $derived(priceUnit(data.currency));
  const numFmt = $derived(new Intl.NumberFormat(locale, { minimumFractionDigits: unit.decimals, maximumFractionDigits: unit.decimals }));
  const fmt = (v: number) => numFmt.format(v * unit.factor);
  const axisFmt = $derived(new Intl.NumberFormat(locale, { maximumFractionDigits: unit.factor === 1 ? 2 : 0 }));
  const fmtAxis = (v: number) => axisFmt.format(v);

  const stepMs = $derived(data.stepSec * 1000);
  const th = $derived(thresholds(data.slots));

  const cur = $derived(currentSlot(data.slots, now, data.stepSec));
  const level = $derived(cur ? levelOf(cur.price, th) : undefined);

  const dayKey = $derived(new Intl.DateTimeFormat('en-CA', { timeZone: tz }));
  const today = $derived(stats(data.slots.filter((s) => dayKey.format(s.t * 1000) === dayKey.format(now))));
  const timeFmt = $derived(new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit', hour12: false, timeZone: tz }));
  const hourFmt = $derived(new Intl.DateTimeFormat(locale, { hour: '2-digit', hour12: false, timeZone: tz }));

  const points = $derived(data.slots.map((s) => ({ t: s.t * 1000, v: s.price * unit.factor })));

  // finger readout, same look as the other charts
  let inspected = $state<Inspected | null>(null);
  const readoutFmt = $derived(
    new Intl.DateTimeFormat(locale, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false, timeZone: tz }),
  );
  const priceName = $derived(i18n.t('tibber.price'));

  const compact = $derived(size.height < 200);
  const showChart = $derived(size.height >= 240);
  const priceSize = $derived(compact ? 44 : Math.min(72, Math.max(48, size.height * 0.15)));
</script>

<div class="tibber" class:compact>
  <header>
    <div class="now">
      <div class="price" style:font-size="{priceSize}px">
        {#if cur}{fmt(cur.price)}{:else}–{/if}<span class="unit">{unit.label}</span>
      </div>
      <div class="sub">
        {#if level}<span class="chip {level}">{i18n.t(`tibber.level.${level}`)}</span>{/if}
        {#if data.label}<span class="label">{data.label}</span>{/if}
      </div>
    </div>
    {#if today && !compact}
      <dl class="facts">
        <div><dt>{i18n.t('tibber.low')}</dt><dd>{fmt(today.min.price)} <small>{timeFmt.format(today.min.t * 1000)}</small></dd></div>
        <div><dt>{i18n.t('tibber.avg')}</dt><dd>{fmt(today.avg)}</dd></div>
        <div><dt>{i18n.t('tibber.high')}</dt><dd>{fmt(today.max.price)} <small>{timeFmt.format(today.max.t * 1000)}</small></dd></div>
      </dl>
    {/if}
  </header>

  {#if showChart}
    <div class="chart">
      <PriceChart {points} {stepMs} {now} formatX={(t) => hourFmt.format(t)} formatY={fmtAxis} formatNow={(v) => numFmt.format(v)} name={priceName} bind:inspected />
    </div>
  {/if}
  {#if showChart && inspected}
    <ChartReadout {inspected} formatT={(t) => readoutFmt.format(t)} formatV={(v) => `${numFmt.format(v)} ${unit.label}`} />
  {/if}
</div>

<style>
  .tibber {
    position: relative;
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5);
  }
  .tibber.compact {
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
  .price {
    font-weight: 300;
    line-height: 1;
    letter-spacing: -0.03em;
    white-space: nowrap;
  }
  .unit {
    margin-left: var(--space-2);
    color: var(--muted);
    font-size: var(--text-m);
    font-weight: 400;
    letter-spacing: 0;
  }
  .sub {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-top: var(--space-2);
    color: var(--muted);
    min-width: 0;
  }
  .label {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chip {
    padding: 2px var(--space-3);
    border-radius: 999px;
    font-size: var(--text-s);
    background: var(--surface-2);
    color: var(--muted);
  }
  .facts {
    margin: 0 0 0 auto;
    display: flex;
    gap: var(--space-4);
    text-align: right;
    font-size: var(--text-s);
  }
  .facts dt {
    color: var(--muted);
  }
  .facts dd {
    margin: 0;
    font-weight: 600;
  }
  .facts small {
    color: var(--muted);
    font-weight: 400;
  }
  .chart {
    flex: 1;
    min-height: 90px;
  }
</style>
