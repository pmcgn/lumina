<script lang="ts">
  import { i18n } from '../../lib/i18n.svelte';
  import type { WidgetProps } from '../../lib/registry';
  import { toast } from '../../lib/toast.svelte';
  import OhIcon from '../../components/OhIcon.svelte';
  import type { OhData, OhRow, OhSettings } from './types';

  let { data, settings, readOnly, send }: WidgetProps<OhData, OhSettings> = $props();
  const t = i18n.t;

  // Optimistic state: what the user asked for, until openHAB confirms it via the
  // event stream (or a few seconds pass and we fall back to the real state).
  let wanted = $state<Record<string, boolean | string | number>>({});
  const timers = new Map<string, number>();

  function expect(id: string, value: boolean | string | number) {
    wanted[id] = value;
    window.clearTimeout(timers.get(id));
    timers.set(id, window.setTimeout(() => clear(id), 5000));
  }
  function clear(id: string) {
    window.clearTimeout(timers.get(id));
    timers.delete(id);
    delete wanted[id];
  }
  // Server confirmed: forget the expectation.
  $effect(() => {
    for (const row of data.rows) {
      const w = wanted[row.id];
      if (w === undefined) continue;
      if ((row.kind === 'switch' && row.on === w) || (row.kind === 'choice' && row.active === w)) clear(row.id);
      if (row.kind === 'slider' && typeof w === 'number' && row.level !== undefined && Math.abs(row.level - w) <= (row.step ?? 1) / 2) clear(row.id);
    }
  });
  $effect(() => () => timers.forEach((id) => window.clearTimeout(id)));

  const isOn = (row: OhRow) => (typeof wanted[row.id] === 'boolean' ? (wanted[row.id] as boolean) : !!row.on);
  const activeCmd = (row: OhRow) => (typeof wanted[row.id] === 'string' ? (wanted[row.id] as string) : row.active);
  const pending = (row: OhRow) => wanted[row.id] !== undefined;
  const interactive = (row: OhRow) => !readOnly && !row.locked;

  async function toggle(row: OhRow) {
    if (!interactive(row)) return;
    const next = !isOn(row);
    expect(row.id, next);
    const r = await send('command', { widget: row.id, command: next ? 'ON' : 'OFF' });
    if (!r.ok) {
      clear(row.id);
      toast.show({ message: t('openhab.failed', { label: row.label }), kind: 'error' });
    }
  }

  // Sliders: show the value live while dragging, send it once when released.
  let drag = $state<Record<string, number>>({});
  const levelOf = (row: OhRow): number | undefined => {
    if (drag[row.id] !== undefined) return drag[row.id];
    const w = wanted[row.id];
    return typeof w === 'number' ? w : row.level;
  };
  const fmt = $derived(new Intl.NumberFormat(i18n.locale, { maximumFractionDigits: 2 }));
  function sliderText(row: OhRow): string {
    const v = levelOf(row);
    if (v === undefined) return row.value || '–';
    // keep the unit that openHAB's own label uses ("%", "°C" ...)
    let unit = (row.value ?? '').replace(/^[-\d.,\s]+/, '');
    if (!unit && row.shutter) unit = '%';
    return `${fmt.format(v)}${unit ? ' ' + unit : ''}`;
  }
  const pct = (row: OhRow) => {
    const v = levelOf(row) ?? row.min ?? 0;
    const lo = row.min ?? 0;
    const hi = row.max ?? 100;
    return `${Math.min(100, Math.max(0, ((v - lo) / (hi - lo)) * 100))}%`;
  };
  function dragging(row: OhRow, e: Event) {
    drag[row.id] = Number((e.currentTarget as HTMLInputElement).value);
  }
  async function release(row: OhRow, e: Event) {
    const value = Number((e.currentTarget as HTMLInputElement).value);
    delete drag[row.id];
    if (!interactive(row)) return;
    expect(row.id, value);
    const r = await send('command', { widget: row.id, command: String(value) });
    if (!r.ok) {
      clear(row.id);
      toast.show({ message: t('openhab.failed', { label: row.label }), kind: 'error' });
    }
  }
  async function shutter(row: OhRow, command: 'UP' | 'STOP' | 'DOWN') {
    if (!interactive(row)) return;
    const r = await send('command', { widget: row.id, command });
    if (!r.ok) toast.show({ message: t('openhab.failed', { label: row.label }), kind: 'error' });
  }

  async function choose(row: OhRow, command: string) {
    if (!interactive(row) || activeCmd(row) === command) return;
    expect(row.id, command);
    const r = await send('command', { widget: row.id, command });
    if (!r.ok) {
      clear(row.id);
      toast.show({ message: t('openhab.failed', { label: row.label }), kind: 'error' });
    }
  }
</script>

<div class="oh">
  <header><h2>{settings.title ?? data.title}</h2></header>

  <div class="grid">
    {#each data.rows as row (row.id)}
      {#if row.kind === 'frame'}
        <h3 class="frame">{row.label}</h3>
      {:else if row.kind === 'switch'}
        <button
          class="tile switch"
          class:on={isOn(row)}
          class:pending={pending(row)}
          class:static={!interactive(row)}
          role="switch"
          aria-checked={isOn(row)}
          aria-label={row.label}
          disabled={!interactive(row)}
          onclick={() => toggle(row)}
        >
          <span class="icon"><OhIcon name={row.icon ?? ''} size={26} /></span>
          <span class="label">{row.label}</span>
          <span class="state">{isOn(row) ? t('openhab.on') : t('openhab.off')}</span>
        </button>
      {:else if row.kind === 'slider'}
        <div class="tile slider" class:pending={pending(row)}>
          <span class="head">
            <span class="icon small"><OhIcon name={row.icon ?? ''} size={20} /></span>
            <span class="label">{row.label}</span>
            <span class="num">{sliderText(row)}</span>
          </span>
          <input
            type="range"
            min={row.min}
            max={row.max}
            step={row.step}
            value={levelOf(row) ?? row.min ?? 0}
            style:--pct={pct(row)}
            disabled={!interactive(row)}
            aria-label={row.label}
            oninput={(e) => dragging(row, e)}
            onchange={(e) => release(row, e)}
          />
          {#if row.shutter && interactive(row)}
            <span class="shutter">
              <button aria-label={t('openhab.up')} onclick={() => shutter(row, 'UP')}>▲</button>
              <button aria-label={t('openhab.stop')} onclick={() => shutter(row, 'STOP')}>■</button>
              <button aria-label={t('openhab.down')} onclick={() => shutter(row, 'DOWN')}>▼</button>
            </span>
          {/if}
        </div>
      {:else if row.kind === 'choice'}
        <div class="tile choice" class:pending={pending(row)}>
          <span class="head"><span class="icon small"><OhIcon name={row.icon ?? ''} size={20} /></span><span class="label">{row.label}</span></span>
          <span class="segments">
            {#each row.mappings ?? [] as m (m.command)}
              <button class:active={activeCmd(row) === m.command} disabled={!interactive(row)} onclick={() => choose(row, m.command)}>{m.label}</button>
            {/each}
          </span>
        </div>
      {:else}
        <div class="tile text">
          <span class="head"><span class="icon small"><OhIcon name={row.icon ?? ''} size={20} /></span><span class="label">{row.label}</span></span>
          <span class="value">{row.value || '–'}</span>
        </div>
      {/if}
    {/each}
  </div>
</div>

<style>
  .oh {
    height: 100%;
    display: flex;
    flex-direction: column;
    padding: var(--space-4) var(--space-4) var(--space-4) var(--space-5);
  }
  header {
    margin-bottom: var(--space-3);
  }
  h2 {
    margin: 0;
    font-size: var(--text-l);
    font-weight: 600;
  }
  .grid {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    -webkit-overflow-scrolling: touch;
    touch-action: pan-y;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    grid-auto-rows: min-content;
    gap: var(--space-3);
    padding-right: var(--space-2);
    align-content: start;
  }
  .frame {
    grid-column: 1 / -1;
    margin: var(--space-2) 0 0;
    color: var(--muted);
    font-size: var(--text-s);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .frame:first-child {
    margin-top: 0;
  }

  .tile {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-height: 92px;
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-s);
    background: var(--surface-2);
    color: var(--text);
    text-align: left;
    font: inherit;
  }
  .label {
    font-weight: 500;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .icon {
    width: 40px;
    height: 40px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    background: var(--border);
    color: var(--muted);
    transition: background var(--motion), color var(--motion);
  }
  .icon.small {
    width: 28px;
    height: 28px;
    background: transparent;
  }
  .head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--muted);
    font-size: var(--text-s);
  }

  /* switch tile */
  .switch {
    cursor: pointer;
    transition: background var(--motion), border-color var(--motion), transform 120ms;
  }
  .switch .state {
    margin-top: auto;
    color: var(--muted);
    font-size: var(--text-s);
  }
  .switch.on {
    background: var(--accent-soft);
    border-color: var(--accent);
  }
  .switch.on .icon {
    background: var(--accent);
    color: #fff;
  }
  .switch.on .state {
    color: var(--accent);
    font-weight: 600;
  }
  .switch:active:not(:disabled) {
    transform: scale(0.96);
  }
  .switch.pending .icon {
    animation: pulse 0.9s ease-in-out infinite;
  }
  .switch.static {
    cursor: default;
  }
  .switch:disabled {
    color: var(--text);
  }
  @media (hover: hover) {
    .switch:hover:not(:disabled):not(.on) {
      border-color: var(--accent);
      background: var(--surface);
    }
    .switch:hover:not(:disabled).on {
      filter: brightness(1.08);
    }
  }
  @keyframes pulse {
    50% {
      opacity: 0.45;
    }
  }

  /* choice tile (sitemap mappings) */
  .choice {
    grid-column: span 2;
  }
  .segments {
    display: flex;
    gap: var(--space-1);
    margin-top: auto;
  }
  .segments button {
    flex: 1;
    min-height: 44px;
    padding: 0 var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-s);
    background: transparent;
    cursor: pointer;
    transition: background var(--motion), transform 120ms;
  }
  .segments button.active {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
    font-weight: 600;
  }
  .segments button:active:not(:disabled) {
    transform: scale(0.96);
  }
  .segments button:disabled {
    cursor: default;
  }
  @media (hover: hover) {
    .segments button:hover:not(:disabled):not(.active) {
      background: var(--surface);
      border-color: var(--accent);
    }
  }
  .choice.pending .segments button.active {
    opacity: 0.7;
  }

  /* slider tile */
  .slider {
    grid-column: span 2;
  }
  .slider .head .label {
    flex: 1;
    min-width: 0;
    color: var(--text);
    font-size: var(--text-m);
    white-space: nowrap;
  }
  .slider .num {
    color: var(--text);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .slider input[type='range'] {
    -webkit-appearance: none;
    appearance: none;
    width: 100%;
    height: 36px;
    margin: 0;
    background: transparent;
    cursor: pointer;
    touch-action: none; /* dragging the slider must not scroll or swipe */
  }
  .slider input[type='range']:disabled {
    cursor: default;
    opacity: 0.6;
  }
  .slider input[type='range']::-webkit-slider-runnable-track {
    height: 8px;
    border-radius: 4px;
    background: linear-gradient(to right, var(--accent) var(--pct), var(--border) var(--pct));
  }
  .slider input[type='range']::-moz-range-track {
    height: 8px;
    border-radius: 4px;
    background: linear-gradient(to right, var(--accent) var(--pct), var(--border) var(--pct));
  }
  .slider input[type='range']::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 28px;
    height: 28px;
    margin-top: -10px;
    border-radius: 50%;
    border: 3px solid var(--accent);
    background: #fff;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.35);
  }
  .slider input[type='range']::-moz-range-thumb {
    width: 22px;
    height: 22px;
    border-radius: 50%;
    border: 3px solid var(--accent);
    background: #fff;
  }
  .slider.pending .num {
    color: var(--accent);
  }
  .shutter {
    display: flex;
    gap: var(--space-2);
  }
  .shutter button {
    flex: 1;
    min-height: 44px;
    border: 1px solid var(--border);
    border-radius: var(--radius-s);
    background: transparent;
    cursor: pointer;
    transition: background var(--motion), transform 120ms;
  }
  .shutter button:active {
    transform: scale(0.96);
    background: var(--accent-soft);
  }
  @media (hover: hover) {
    .shutter button:hover {
      background: var(--surface);
      border-color: var(--accent);
    }
  }

  /* value tile */
  .text .value {
    margin-top: auto;
    font-size: var(--text-l);
    font-weight: 300;
    letter-spacing: -0.01em;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
