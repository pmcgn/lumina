<script lang="ts">
  import { toast } from '../lib/toast.svelte';
</script>

<div class="host" aria-live="polite">
  {#each toast.items as t (t.id)}
    <div class="toast" class:error={t.kind === 'error'} role="status">
      <span class="msg">{t.message}</span>
      {#if t.actionLabel}
        <button
          onclick={() => {
            t.onAction?.();
            toast.dismiss(t.id);
          }}>{t.actionLabel}</button
        >
      {/if}
    </div>
  {/each}
</div>

<style>
  .host {
    position: fixed;
    left: 50%;
    bottom: var(--space-5);
    transform: translateX(-50%);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    align-items: center;
    z-index: 20;
    pointer-events: none;
  }
  .toast {
    pointer-events: auto;
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-2) var(--space-2) var(--space-5);
    border-radius: 999px;
    background: var(--text);
    color: var(--bg);
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.35);
    max-width: 90vw;
  }
  .toast.error {
    background: var(--danger);
    color: #fff;
    padding-right: var(--space-5);
  }
  .msg {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  button {
    min-height: 44px;
    padding: 0 var(--space-4);
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--accent);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-size: var(--text-s);
    cursor: pointer;
    transition: background var(--motion), transform 120ms;
  }
  button:active {
    transform: scale(0.94);
    background: var(--accent-soft-strong);
  }
  @media (hover: hover) {
    button:hover {
      background: var(--accent-soft);
    }
  }
</style>
