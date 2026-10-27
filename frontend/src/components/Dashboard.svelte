<script lang="ts">
  import { fade } from 'svelte/transition';
  import { connection } from '../lib/connection.svelte';
  import type { ClientConfig } from '../lib/protocol';
  import WidgetShell from './WidgetShell.svelte';

  let { config }: { config: ClientConfig } = $props();

  let current = $state(0);
  const page = $derived(config.pages[Math.min(current, config.pages.length - 1)]);

  // Only widgets on the visible page receive data.
  $effect(() => {
    connection.setSubscriptions(page.widgets.map((w) => w.id));
  });

  // Auto-rotation: cycle through the pages; any touch pauses it for a while.
  const rotate = $derived(config.display.rotate);
  let pausedUntil = 0;
  function interacted() {
    pausedUntil = Date.now() + rotate.pauseSeconds * 1000;
  }
  function show(i: number) {
    current = i;
    interacted();
  }
  $effect(() => {
    const pages = config.pages.length;
    if (!rotate.enabled || pages < 2) return;
    const timer = window.setInterval(() => {
      if (Date.now() >= pausedUntil) current = (current + 1) % pages;
    }, rotate.intervalSeconds * 1000);
    return () => window.clearInterval(timer);
  });

  // Swipe to change page (but not while dragging a slider).
  let startX = 0;
  let startY = 0;
  let swiping = true;
  function onTouchStart(e: TouchEvent) {
    swiping = !(e.target as Element).closest('input[type="range"], [data-no-swipe]');
    startX = e.touches[0].clientX;
    startY = e.touches[0].clientY;
  }
  function onTouchEnd(e: TouchEvent) {
    if (!swiping) return;
    const dx = e.changedTouches[0].clientX - startX;
    const dy = e.changedTouches[0].clientY - startY;
    if (Math.abs(dx) < 80 || Math.abs(dx) < Math.abs(dy) * 1.5) return;
    const next = current + (dx < 0 ? 1 : -1);
    if (next >= 0 && next < config.pages.length) show(next);
  }
</script>

<div class="dashboard" role="presentation" onpointerdowncapture={interacted}>
  {#key page.id}
    <div
      class="grid"
      role="presentation"
      in:fade={{ duration: config.display.lite ? 0 : 300 }}
      style:grid-template-columns="repeat({config.grid.columns}, minmax(0, 1fr))"
      style:grid-template-rows="repeat({config.grid.rows}, minmax(0, 1fr))"
      ontouchstart={onTouchStart}
      ontouchend={onTouchEnd}
    >
      {#each page.widgets as w (w.id)}
        <WidgetShell widget={w} />
      {/each}
    </div>
  {/key}

  {#if config.pages.length > 1}
    <nav class="tabs">
      {#each config.pages as p, i (p.id)}
        <button class:active={i === current} onclick={() => show(i)}>{p.title}</button>
      {/each}
    </nav>
  {/if}
</div>

<style>
  .dashboard {
    height: 100%;
    display: flex;
    flex-direction: column;
    padding: var(--space-4);
    gap: var(--space-3);
  }
  .grid {
    flex: 1;
    min-height: 0;
    display: grid;
    gap: var(--space-4);
  }
  .tabs {
    display: flex;
    justify-content: center;
    gap: var(--space-2);
  }
  .tabs button {
    min-height: 44px;
    padding: 0 var(--space-5);
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--muted);
    transition: background var(--motion), color var(--motion);
  }
  .tabs button {
    cursor: pointer;
  }
  .tabs button:active {
    transform: scale(0.95);
  }
  @media (hover: hover) {
    .tabs button:not(.active):hover {
      background: var(--surface);
      color: var(--text);
    }
  }
  .tabs button.active {
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow);
  }
</style>
