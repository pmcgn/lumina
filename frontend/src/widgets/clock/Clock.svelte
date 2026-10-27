<script lang="ts">
  import { i18n } from '../../lib/i18n.svelte';
  import type { WidgetProps } from '../../lib/registry';
  import type { Data, Settings } from './model.gen';

  let { data, settings, size }: WidgetProps<Data, Settings> = $props();

  let now = $state(new Date());

  // Tick locally (no server traffic), aligned to the second/minute boundary.
  $effect(() => {
    const step = settings.showSeconds ? 1000 : 60_000;
    let timer: number;
    const tick = () => {
      now = new Date();
      timer = window.setTimeout(tick, step - (Date.now() % step) + 5);
    };
    tick();
    return () => window.clearTimeout(timer);
  });

  const tz = $derived(data.timezone || undefined);
  const time = $derived(
    new Intl.DateTimeFormat(i18n.locale, {
      hour: '2-digit',
      minute: '2-digit',
      second: settings.showSeconds ? '2-digit' : undefined,
      hour12: settings.hour12 ?? false,
      timeZone: tz,
    }).format(now),
  );
  const date = $derived(
    new Intl.DateTimeFormat(i18n.locale, { weekday: 'long', day: 'numeric', month: 'long', timeZone: tz }).format(now),
  );
  const week = $derived(isoWeek(now, tz));

  function isoWeek(d: Date, timeZone?: string): number {
    // Calendar date in the target timezone, then the standard ISO-8601 algorithm.
    const parts = new Intl.DateTimeFormat('en-CA', { year: 'numeric', month: 'numeric', day: 'numeric', timeZone }).formatToParts(d);
    const p = (t: string) => Number(parts.find((x) => x.type === t)!.value);
    const date = new Date(Date.UTC(p('year'), p('month') - 1, p('day')));
    const day = date.getUTCDay() || 7;
    date.setUTCDate(date.getUTCDate() + 4 - day);
    const yearStart = Date.UTC(date.getUTCFullYear(), 0, 1);
    return Math.ceil(((date.getTime() - yearStart) / 86_400_000 + 1) / 7);
  }

  // Scale the digits with the card (about 28% of the height, limited by width).
  const fontSize = $derived(Math.max(28, Math.min(size.height * 0.34, size.width / (settings.showSeconds ? 5 : 3.4))));
</script>

<div class="clock">
  {#if settings.label}<div class="label">{settings.label}</div>{/if}
  <div class="time" style:font-size="{fontSize}px">{time}</div>
  <div class="date">{date} · {i18n.t('clock.week', { week })}</div>
</div>

<style>
  .clock {
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--space-3);
    text-align: center;
  }
  .time {
    font-weight: 300;
    line-height: 1.05;
    letter-spacing: -0.02em;
    font-variant-numeric: tabular-nums;
  }
  .date {
    margin-top: var(--space-2);
    color: var(--muted);
    font-size: var(--text-m);
  }
  .label {
    color: var(--accent);
    font-size: var(--text-s);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    margin-bottom: var(--space-1);
  }
</style>
