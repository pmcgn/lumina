<script lang="ts">
  import type { Inspected } from './TimeChart.svelte';

  interface Props {
    inspected: Inspected;
    formatT: (t: number) => string;
    formatV: (v: number, name: string) => string;
  }

  let { inspected, formatT, formatV }: Props = $props();
</script>

<div class="readout">
  <span class="when">{formatT(inspected.t)}</span>
  {#each inspected.rows as r (r.name)}
    <span class="item"><i style:background={r.color}></i><span class="n">{r.name}</span><b>{formatV(r.v, r.name)}</b></span>
  {/each}
</div>

<style>
  .readout {
    position: absolute;
    z-index: 1;
    top: var(--space-2);
    left: var(--space-3);
    right: var(--space-3);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-4);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-s);
    background: var(--surface-2);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
    font-size: var(--text-s);
    pointer-events: none;
  }
  .when {
    color: var(--muted);
  }
  .item {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  i {
    flex: none;
    width: 9px;
    height: 9px;
    border-radius: 50%;
  }
  .n {
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 14em;
  }
</style>
