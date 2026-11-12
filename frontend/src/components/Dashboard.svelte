<script lang="ts">
  import { cubicOut } from 'svelte/easing';
  import { blur, fade, fly, scale, slide, type TransitionConfig } from 'svelte/transition';
  import { connection } from '../lib/connection.svelte';
  import type { ClientConfig } from '../lib/protocol';
  import WidgetShell from './WidgetShell.svelte';

  let { config }: { config: ClientConfig } = $props();

  let current = $state(0);
  const page = $derived(config.pages[Math.min(current, config.pages.length - 1)]);

  // Page transition (display.transition); lite mode switches it off.
  const transition = $derived(config.display.transition);
  const duration = $derived(config.display.lite ? 0 : transition.durationMs);
  // 1 = moving to a later page, -1 = to an earlier one (for fly).
  let direction = 1;
  let lastIndex = 0;
  $effect.pre(() => {
    if (current !== lastIndex) direction = current > lastIndex ? 1 : -1;
    lastIndex = current;
  });
  function pageIn(node: Element): TransitionConfig {
    switch (transition.type) {
      case 'fly':
        return fly(node, { x: `${direction * 100}%`, duration, easing: cubicOut });
      case 'slide':
        return slide(node, { axis: 'x', duration, easing: cubicOut });
      case 'blur':
        return blur(node, { amount: 12, duration, easing: cubicOut });
      case 'scale':
        return scale(node, { start: 0.96, duration, easing: cubicOut });
      default: // fade, crossfade
        return fade(node, { duration });
    }
  }
  // 'fade' only fades the new page in; the others also animate the old page out.
  function pageOut(node: Element): TransitionConfig {
    switch (transition.type) {
      case 'fly':
        return fly(node, { x: `${-direction * 100}%`, duration, easing: cubicOut });
      case 'slide':
        return slide(node, { axis: 'x', duration, easing: cubicOut });
      case 'blur':
        return blur(node, { amount: 12, duration, easing: cubicOut });
      case 'scale':
        return scale(node, { start: 0.96, duration, easing: cubicOut });
      case 'crossfade':
        return fade(node, { duration });
      default:
        return { duration: 0 };
    }
  }

  // Only widgets on the visible page receive data.
  $effect(() => {
    connection.setSubscriptions(page.widgets.map((w) => w.id));
  });

  // Auto-rotation: cycle through the pages; any touch pauses it for a while.
  const rotate = $derived(config.display.rotate);
  let pausedUntil = 0;
  let lastInteraction = Date.now();
  function interacted() {
    pausedUntil = Date.now() + rotate.pauseSeconds * 1000;
    lastInteraction = Date.now();
  }

  // A page flagged as home is returned to after a while without interaction.
  const HOME_AFTER_MS = 30_000;
  const homeIndex = $derived(config.pages.findIndex((p) => p.home));
  $effect(() => {
    if (homeIndex < 0) return;
    const timer = window.setInterval(() => {
      if (current !== homeIndex && Date.now() - lastInteraction >= HOME_AFTER_MS) current = homeIndex;
    }, 1000);
    return () => window.clearInterval(timer);
  });
  function show(i: number) {
    current = i;
    interacted();
  }
  $effect(() => {
    const pages = config.pages.length;
    if (!rotate.enabled || pages < 2 || homeIndex >= 0) return;
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
  <div class="stage" class:clip={transition.type === 'fly' || transition.type === 'slide'}>
  {#key page.id}
    <div
      class="grid"
      role="presentation"
      in:pageIn
      out:pageOut
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
  </div>

  {#if config.pages.length > 1}
    <nav class="tabs">
      {#each config.pages as p, i (p.id)}
        <button class:active={i === current} onclick={() => show(i)}>
          {#if p.home}
            <svg class="home-icon" viewBox="0 0 24 24" aria-label="Home" role="img">
              <path d="M12 3 2 12h3v8h5v-6h4v6h5v-8h3z" fill="currentColor" />
            </svg>
          {/if}
          {p.title}
        </button>
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
  /* Old and new page overlap in the same cell while they transition. */
  .stage {
    flex: 1;
    min-height: 0;
    display: grid;
  }
  .stage.clip {
    overflow: hidden; /* moving pages must not leave the screen area */
  }
  .grid {
    grid-area: 1 / 1;
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
  .home-icon {
    width: 1em;
    height: 1em;
    margin-right: var(--space-1, 4px);
    vertical-align: -0.12em;
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
  /* Over a background picture the muted tab labels would be hard to read. */
  :global(:root[data-background]) .tabs button:not(.active) {
    background: var(--surface-2);
    color: var(--text);
  }
  .tabs button.active {
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow);
  }
</style>
