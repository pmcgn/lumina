<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import { dueInfo } from '../../lib/dates';
  import { i18n } from '../../lib/i18n.svelte';
  import type { WidgetProps } from '../../lib/registry';
  import { toast } from '../../lib/toast.svelte';
  import type { VikunjaData, VikunjaSettings, VikunjaTask } from './types';

  let { data, settings, readOnly, send }: WidgetProps<VikunjaData, VikunjaSettings> = $props();
  const t = i18n.t;

  // Re-evaluate overdue state locally, so a task flips at the moment it is due
  // without waiting for the next server poll.
  let now = $state(new Date());
  $effect(() => {
    const timer = window.setInterval(() => (now = new Date()), 30_000);
    return () => window.clearInterval(timer);
  });

  // Ticked tasks stay visible with a "done" look (pending) and are collapsed a
  // moment after the server confirmed (hidden).
  const pending = new SvelteSet<number>();
  const hidden_ = new SvelteSet<number>();
  $effect(() => {
    const present = new Set(data.tasks.map((x) => x.id));
    for (const id of pending) if (!present.has(id)) pending.delete(id);
    for (const id of hidden_) if (!present.has(id)) hidden_.delete(id);
  });

  const rows = $derived(
    data.tasks
      .filter((task) => !hidden_.has(task.id))
      .map((task) => ({ task, due: task.due ? dueInfo(task.due, now, i18n.locale) : null, completing: pending.has(task.id) })),
  );
  const open = $derived(rows.filter((r) => !r.completing));
  const overdue = $derived(open.filter((r) => r.due?.state === 'overdue').length);
  const dueToday = $derived(open.filter((r) => r.due?.state === 'today').length);
  const hidden = $derived(Math.max(0, data.total - data.tasks.length));
  const showProject = $derived(settings.showProject ?? new Set(data.tasks.map((x) => x.projectId)).size > 1);

  async function complete(task: VikunjaTask) {
    if (readOnly || pending.has(task.id)) return;
    pending.add(task.id);
    const r = await send('complete', { id: task.id });
    if (!r.ok) {
      pending.delete(task.id);
      toast.show({ message: t('vikunja.failed'), kind: 'error' });
      return;
    }
    // Let the "done" look register, then collapse the row. A recurring task
    // stays in the list with its next due date instead.
    const finished = (r.data as { done: boolean }).done;
    window.setTimeout(() => {
      if (finished) hidden_.add(task.id);
      else pending.delete(task.id);
    }, 450);
    toast.show({
      message: t('vikunja.completed', { title: task.title }),
      actionLabel: t('toast.undo'),
      timeout: 6000,
      onAction: () => undo(task),
    });
  }

  async function undo(task: VikunjaTask) {
    const r = await send('uncomplete', { id: task.id, due: task.due });
    pending.delete(task.id);
    hidden_.delete(task.id);
    if (!r.ok) toast.show({ message: t('vikunja.failed'), kind: 'error' });
  }

  // Quick add
  let adding = $state(false);
  let draft = $state('');
  let busy = $state(false);
  async function add(e: Event) {
    e.preventDefault();
    const title = draft.trim();
    if (!title || busy) return;
    busy = true;
    const r = await send('add', { title });
    busy = false;
    if (r.ok) {
      draft = '';
      adding = false;
    } else {
      toast.show({ message: t('vikunja.failed'), kind: 'error' });
    }
  }
  function focusOnMount(node: HTMLInputElement) {
    node.focus();
  }
</script>

<div class="tasks">
  <header>
    <h2>{settings.title ?? t('vikunja.title')}</h2>
    <div class="chips">
      {#if overdue > 0}<span class="chip danger">{t('vikunja.overdue', { n: overdue })}</span>{/if}
      {#if dueToday > 0}<span class="chip warn">{t('vikunja.today', { n: dueToday })}</span>{/if}
    </div>
  </header>

  <ul>
    {#each rows as { task, due, completing } (task.id)}
      <li class:overdue={due?.state === 'overdue'} class:completing>
        {#if task.priority >= 3}<span class="prio" class:urgent={task.priority >= 4} title={t('vikunja.priority')}></span>{/if}
        {#if readOnly}
          <span class="dot" class:overdue={due?.state === 'overdue'}></span>
        {:else}
          <button class="check" disabled={completing} onclick={() => complete(task)} aria-label={t('vikunja.complete', { title: task.title })}>
            <span class="ring" class:overdue={due?.state === 'overdue'}>
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 12.5l4 4 8-9" /></svg>
            </span>
          </button>
        {/if}
        <div class="body">
          <div class="title">{task.title}</div>
          <div class="meta">
            {#if due}<span class="due {due.state}">{due.text}{#if task.recurring} ↻{/if}</span>{/if}
            {#if showProject && task.project}<span>{task.project}</span>{/if}
            {#each task.labels ?? [] as l (l.title)}
              <span class="label" style:--c={l.color ? `#${l.color}` : 'var(--muted)'}>{l.title}</span>
            {/each}
          </div>
        </div>
      </li>
    {:else}
      <li class="empty">{t('vikunja.empty')}</li>
    {/each}
    {#if hidden > 0}<li class="more">{t('vikunja.more', { n: hidden })}</li>{/if}
  </ul>

  {#if data.canAdd && !readOnly}
    <footer>
      {#if adding}
        <form onsubmit={add}>
          <input use:focusOnMount bind:value={draft} placeholder={t('vikunja.addPlaceholder')} maxlength="200" enterkeyhint="done" />
          <button type="submit" class="primary" disabled={busy || !draft.trim()}>{t('vikunja.save')}</button>
          <button
            type="button"
            onclick={() => {
              adding = false;
              draft = '';
            }}>✕</button
          >
        </form>
      {:else}
        <button class="addbtn" onclick={() => (adding = true)}>＋ {t('vikunja.add')}</button>
      {/if}
    </footer>
  {/if}
</div>

<style>
  .tasks {
    height: 100%;
    display: flex;
    flex-direction: column;
    padding: var(--space-4) var(--space-4) var(--space-2) var(--space-5);
  }
  header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
    padding-right: var(--space-2);
    margin-bottom: var(--space-2);
  }
  h2 {
    margin: 0;
    font-size: var(--text-l);
    font-weight: 600;
  }
  .chips {
    display: flex;
    gap: var(--space-2);
  }
  .chip {
    padding: 2px var(--space-3);
    border-radius: 999px;
    font-size: var(--text-s);
    font-weight: 600;
  }
  .chip.danger {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .chip.warn {
    background: var(--warn-soft);
    color: var(--warn);
  }
  ul {
    flex: 1;
    min-height: 0;
    margin: 0;
    padding: 0 var(--space-2) 0 0;
    list-style: none;
    overflow-y: auto;
    -webkit-overflow-scrolling: touch;
    touch-action: pan-y;
  }
  li {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 60px;
    padding-left: var(--space-2);
    border-bottom: 1px solid var(--border);
  }
  li:last-child {
    border-bottom: 0;
  }
  .prio {
    position: absolute;
    left: 0;
    top: 14px;
    bottom: 14px;
    width: 4px;
    border-radius: 2px;
    background: var(--warn);
  }
  .prio.urgent {
    background: var(--danger);
  }
  .check {
    flex: none;
    width: 52px;
    height: 52px;
    display: grid;
    place-items: center;
    padding: 0;
    border: 0;
    background: transparent;
  }
  .ring {
    width: 28px;
    height: 28px;
    box-sizing: border-box;
    display: grid;
    place-items: center;
    border-radius: 50%;
    border: 2px solid var(--muted);
    transition: background var(--motion), border-color var(--motion);
  }
  .ring.overdue {
    border-color: var(--danger);
  }
  .ring svg {
    width: 18px;
    height: 18px;
    fill: none;
    stroke: #fff;
    stroke-width: 3;
    stroke-linecap: round;
    stroke-linejoin: round;
    opacity: 0;
    transition: opacity var(--motion);
  }
  .check {
    cursor: pointer;
    transition: transform 120ms;
  }
  .check:active {
    transform: scale(0.88);
  }
  /* pressed, or completed and waiting for the row to collapse: filled green with a check */
  .check:active .ring,
  .completing .ring {
    background: var(--ok);
    border-color: var(--ok);
  }
  .check:active .ring svg,
  .completing .ring svg {
    opacity: 1;
  }
  /* pointer devices: preview the check on hover (touch screens have no hover) */
  @media (hover: hover) {
    .check:hover .ring {
      border-color: var(--ok);
      background: var(--ok-soft);
    }
    .check:hover .ring svg {
      opacity: 0.9;
      stroke: var(--ok);
    }
  }
  .completing .check {
    pointer-events: none;
  }
  .completing .body {
    opacity: 0.45;
  }
  .completing .title {
    text-decoration: line-through;
  }
  li {
    transition: opacity var(--motion);
  }
  .dot {
    flex: none;
    width: 10px;
    height: 10px;
    margin: 0 21px;
    border-radius: 50%;
    background: var(--muted);
  }
  .dot.overdue {
    background: var(--danger);
  }
  .body {
    flex: 1;
    min-width: 0;
    padding: var(--space-2) 0;
  }
  .title {
    font-weight: 500;
    line-height: 1.25;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    margin-top: 2px;
    color: var(--muted);
    font-size: var(--text-s);
  }
  .due.overdue {
    color: var(--danger);
    font-weight: 600;
  }
  .due.today {
    color: var(--warn);
    font-weight: 600;
  }
  .label {
    padding: 0 var(--space-2);
    border-radius: 999px;
    color: var(--c);
    border: 1px solid var(--c);
    font-size: 0.72rem;
    line-height: 1.5;
  }
  .empty,
  .more {
    justify-content: center;
    color: var(--muted);
    min-height: 48px;
  }
  .empty {
    height: 100%;
    font-size: var(--text-m);
  }
  footer {
    padding: var(--space-2) 0;
  }
  .addbtn {
    cursor: pointer;
    transition: background var(--motion), color var(--motion), border-color var(--motion), transform 120ms;
    width: 100%;
    min-height: 48px;
    border: 1px dashed var(--border);
    border-radius: var(--radius-s);
    background: transparent;
    color: var(--muted);
  }
  .addbtn:active,
  form button:active:not(:disabled) {
    transform: scale(0.97);
    background: var(--surface-2);
  }
  @media (hover: hover) {
    .addbtn:hover {
      color: var(--text);
      border-color: var(--accent);
      background: var(--surface-2);
    }
    form button:hover:not(:disabled):not(.primary) {
      background: var(--border);
    }
    form button.primary:hover:not(:disabled) {
      filter: brightness(1.1);
    }
  }
  form {
    display: flex;
    gap: var(--space-2);
  }
  input {
    flex: 1;
    min-width: 0;
    min-height: 48px;
    padding: 0 var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-s);
    background: var(--surface-2);
    color: var(--text);
    font: inherit;
    -webkit-user-select: text;
    user-select: text;
  }
  form button {
    cursor: pointer;
    transition: background var(--motion), transform 120ms, filter var(--motion);
    min-width: 48px;
    min-height: 48px;
    padding: 0 var(--space-3);
    border: 0;
    border-radius: var(--radius-s);
    background: var(--surface-2);
  }
  form button.primary:active:not(:disabled) {
    background: var(--accent);
    filter: brightness(0.9);
  }
  form button.primary {
    background: var(--accent);
    color: #fff;
    font-weight: 600;
  }
  form button:disabled {
    opacity: 0.5;
  }
</style>
