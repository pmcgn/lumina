<script lang="ts">
  import { connection } from '../lib/connection.svelte';
  import { i18n } from '../lib/i18n.svelte';

  // Alerts also leave on their own at the deadline, so a missed alert_clear cannot leave one stuck.
  let now = $state(Date.now());
  $effect(() => {
    const next = Math.min(...connection.alerts.map((a) => a.deadline));
    if (!Number.isFinite(next)) return;
    const timer = window.setTimeout(() => (now = Date.now()), Math.max(next - Date.now(), 0) + 50);
    return () => window.clearTimeout(timer);
  });

  const visible = $derived(connection.alerts.filter((a) => a.deadline > now));
  const current = $derived(visible.length ? visible[visible.length - 1] : undefined); // the newest one
  const others = $derived(visible.length - 1);

  // Long texts get a smaller type so that they still fit.
  function textSize(text: string): string {
    const n = text.length;
    const vmin = n <= 24 ? 9.5 : n <= 50 ? 8.5 : n <= 100 ? 6.5 : n <= 200 ? 5 : n <= 350 ? 4 : 3.2;
    return `${vmin}vmin`;
  }

  let busy = $state(false);
  let failed = $state(false);
  $effect(() => {
    void current?.id; // a different alert starts with a clean button
    busy = false;
    failed = false;
  });

  async function confirm() {
    if (!current || busy) return;
    busy = true;
    failed = false;
    const res = await connection.confirmAlert(current.id);
    busy = false;
    failed = !res.ok;
  }
</script>

{#if current}
  {#key current.id}
    <div class="scrim" class:orange={current.color === 'orange'}>
      <div class="card" role="alertdialog" aria-live="assertive" aria-label={i18n.t('alert.' + current.color)}>
        <div class="row">
          <div class="badge" aria-hidden="true">
            <svg viewBox="0 0 24 24" width="76" height="76" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
              <path d="M12 9v4M12 17h.01" />
            </svg>
          </div>

          <div class="main">
            <span class="label">{i18n.t('alert.' + current.color)}</span>
            <p class="text" style:font-size={textSize(current.text)}>{current.text}</p>
            {#if current.confirmable}
              <div class="actions">
                <button type="button" class="confirm" disabled={busy} onclick={confirm}>{i18n.t('alert.confirm')}</button>
                {#if failed}<span class="failed" role="status">{i18n.t('alert.failed')}</span>{/if}
              </div>
            {/if}
          </div>

          <div class="side">
            {#if others > 0}<span class="more">{i18n.t('alert.more', { n: others })}</span>{/if}
          </div>
        </div>

        <div class="timer" aria-hidden="true">
          <div class="fill" style:animation-duration="{Math.max(current.deadline - Date.now(), 0)}ms"></div>
        </div>
      </div>
    </div>
  {/key}
{/if}

<style>
  .scrim {
    --c: var(--danger);
    --c-soft: var(--danger-soft);
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 40; /* above night mode (30) and toasts (20) */
    display: grid;
    place-items: center;
    padding: 0;
    background: radial-gradient(circle at 50% 45%, var(--c-soft), transparent 65%), rgba(8, 10, 16, 0.78);
    animation: fade-in 220ms ease-out;
  }
  .scrim.orange {
    --c: var(--orange);
    --c-soft: var(--orange-soft);
  }

  .card {
    position: relative;
    width: 100%;
    max-height: 100%;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    background: var(--c);
    color: #fff;
    text-align: center;
    box-shadow: 0 30px 80px rgba(0, 0, 0, 0.55), 0 0 120px var(--c-soft), 0 0 60px var(--c-soft);
    animation: pop-in 280ms cubic-bezier(0.2, 0.9, 0.3, 1.2);
  }

  .row {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: 220px 1fr 220px; /* icon | text | balance, so the text is truly centered */
    align-items: center;
    padding: var(--space-5) 0;
  }
  .main {
    min-height: 0;
    max-height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
  }
  .side {
    position: relative;
    align-self: start;
    justify-self: end;
    padding-right: var(--space-4);
  }
  .badge {
    position: relative;
    width: 124px;
    height: 124px;
    justify-self: center;
    display: grid;
    place-items: center;
    border-radius: 50%;
    background: #fff;
    color: var(--c);
  }
  .badge::after {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    border-radius: 50%;
    border: 3px solid #fff;
    animation: pulse 1.8s ease-out infinite;
  }
  .label {
    font-size: var(--text-l);
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
  }
  .more {
    display: block;
    padding: var(--space-1) var(--space-3);
    border-radius: 999px;
    background: rgba(0, 0, 0, 0.25);
    color: #fff;
    font-size: var(--text-s);
    font-weight: 600;
  }

  .text {
    margin: 0;
    padding: var(--space-3) var(--space-4) var(--space-4);
    overflow-y: auto;
    font-weight: 700;
    line-height: 1.15;
    letter-spacing: -0.01em;
    overflow-wrap: break-word;
    word-break: break-word;
    white-space: pre-line;
  }

  .actions {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: var(--space-2) 0 0;
  }
  .confirm {
    min-height: 68px;
    padding: 0 48px;
    border: 0;
    border-radius: 999px;
    background: #fff;
    color: var(--c);
    font: inherit;
    font-size: var(--text-l);
    font-weight: 700;
    letter-spacing: 0.04em;
    cursor: pointer;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.25);
    transition: transform 120ms, opacity var(--motion);
  }
  .confirm:active {
    transform: scale(0.96);
  }
  .confirm:disabled {
    opacity: 0.6;
  }
  .failed {
    margin-top: var(--space-3);
    font-weight: 600;
  }

  .timer {
    height: 8px;
    flex: none;
    background: rgba(0, 0, 0, 0.25);
  }
  .fill {
    height: 100%;
    background: #fff;
    transform-origin: left center;
    animation: drain linear forwards;
  }

  @keyframes fade-in {
    from {
      opacity: 0;
    }
  }
  @keyframes pop-in {
    from {
      opacity: 0;
      transform: scale(0.9);
    }
  }
  @keyframes pulse {
    from {
      transform: scale(1);
      opacity: 0.7;
    }
    to {
      transform: scale(1.5);
      opacity: 0;
    }
  }
  @keyframes drain {
    to {
      transform: scaleX(0);
    }
  }

  @media (hover: hover) {
    .confirm:hover {
      opacity: 0.9;
    }
  }
  :global(:root[data-lite]) .scrim,
  :global(:root[data-lite]) .card,
  :global(:root[data-lite]) .badge::after {
    animation: none;
  }
</style>
