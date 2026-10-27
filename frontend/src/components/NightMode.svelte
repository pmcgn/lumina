<script lang="ts">
  import { i18n } from '../lib/i18n.svelte';
  import type { DisplayConfig } from '../lib/protocol';
  import { inWindow } from '../lib/schedule';

  let { night }: { night: DisplayConfig['night'] } = $props();

  let now = $state(new Date());
  $effect(() => {
    const timer = window.setInterval(() => (now = new Date()), 5000);
    return () => window.clearInterval(timer);
  });

  const isNight = $derived(inWindow(now, night.from, night.to));

  // Clock mode: a touch shows the dashboard for a while, every further touch extends it.
  let awakeUntil = $state(0);
  const awake = $derived(now.getTime() < awakeUntil);
  const asleep = $derived(isNight && night.mode === 'clock' && !awake);

  function wake() {
    awakeUntil = Date.now() + night.wakeSeconds * 1000;
    now = new Date();
  }
  function extend() {
    if (isNight && night.mode === 'clock' && awake) awakeUntil = Date.now() + night.wakeSeconds * 1000;
  }

  // Slow drift of the clock position so nothing burns into the screen.
  const minute = $derived(now.getHours() * 60 + now.getMinutes());
  const shiftX = $derived(((minute * 37) % 41) - 20);
  const shiftY = $derived(((minute * 53) % 41) - 20);

  const time = $derived(new Intl.DateTimeFormat(i18n.locale, { hour: '2-digit', minute: '2-digit' }).format(now));
  const date = $derived(new Intl.DateTimeFormat(i18n.locale, { weekday: 'long', day: 'numeric', month: 'long' }).format(now));
</script>

<svelte:window onpointerdown={extend} />

{#if isNight && night.mode === 'dim'}
  <div class="dim" style:opacity={night.dim} aria-hidden="true"></div>
{/if}

{#if asleep}
  <div class="sleep" role="button" tabindex="-1" aria-label="wake" onpointerdown={wake}>
    <div class="clock" style:transform="translate({shiftX}px, {shiftY}px)">
      <div class="time">{time}</div>
      <div class="date">{date}</div>
    </div>
  </div>
{/if}

<style>
  .dim {
    position: fixed;
    inset: 0;
    background: #000;
    pointer-events: none; /* the dashboard stays usable, just darker */
    z-index: 15;
    transition: opacity 2s;
  }
  .sleep {
    position: fixed;
    inset: 0;
    z-index: 30;
    display: grid;
    place-items: center;
    background: #000;
  }
  .clock {
    text-align: center;
    color: #6b7280;
  }
  .time {
    font-size: min(28vw, 28vh);
    font-weight: 200;
    line-height: 1;
    letter-spacing: -0.03em;
    font-variant-numeric: tabular-nums;
  }
  .date {
    margin-top: 1.2vh;
    font-size: min(4vw, 4vh);
    color: #4b5563;
  }
</style>
