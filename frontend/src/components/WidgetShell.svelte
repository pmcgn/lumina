<script lang="ts">
  import { onMount } from 'svelte';
  import { connection } from '../lib/connection.svelte';
  import { i18n } from '../lib/i18n.svelte';
  import { loadWidget } from '../lib/registry';
  import type { WidgetConfig } from '../lib/protocol';

  let { widget }: { widget: WidgetConfig } = $props();

  const ws = $derived(connection.widgets.get(widget.id));
  const status = $derived(ws?.status ?? 'loading');
  const loader = $derived(loadWidget(widget.type));

  // Container size in pixels, so widgets can adapt without container queries.
  let el: HTMLElement;
  let px = $state({ width: 0, height: 0 });
  onMount(() => {
    const measure = () => (px = { width: el.clientWidth, height: el.clientHeight });
    measure();
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  });

  const size = $derived({ w: widget.pos.w, h: widget.pos.h, ...px });
  const staleSince = $derived(
    ws?.updatedAt
      ? new Date(ws.updatedAt).toLocaleTimeString(i18n.locale, { hour: '2-digit', minute: '2-digit' })
      : '',
  );
</script>

<section
  class="card"
  bind:this={el}
  style:grid-column="{widget.pos.x + 1} / span {widget.pos.w}"
  style:grid-row="{widget.pos.y + 1} / span {widget.pos.h}"
>
  {#if loader && ws?.data !== undefined}
    {#await loader then Widget}
      <Widget
        id={widget.id}
        data={ws.data}
        settings={widget.settings}
        {status}
        readOnly={widget.readOnly}
        {size}
        send={(name: string, params?: unknown) => connection.send(widget.id, name, params)}
      />
    {/await}
  {:else if status === 'error'}
    <div class="note error">{i18n.t('status.error')}: {ws?.error}</div>
  {:else}
    <div class="note">{i18n.t('status.loading')}</div>
  {/if}

  {#if status === 'stale'}
    <div class="badge" title={ws?.error}>{i18n.t('status.stale', { time: staleSince })}</div>
  {/if}
</section>

<style>
  .card {
    position: relative;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    border-radius: var(--radius);
    background: var(--surface);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
  }
  .note {
    height: 100%;
    display: grid;
    place-items: center;
    padding: var(--space-4);
    text-align: center;
    color: var(--muted);
    font-size: var(--text-s);
  }
  .note.error {
    color: var(--danger);
  }
  .badge {
    position: absolute;
    top: var(--space-2);
    right: var(--space-2);
    padding: 2px var(--space-2);
    border-radius: 999px;
    background: var(--warn);
    color: #1a1400;
    font-size: 0.7rem;
  }
</style>
