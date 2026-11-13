<script lang="ts">
  import { onMount } from 'svelte';
  import { connection } from './lib/connection.svelte';
  import { i18n } from './lib/i18n.svelte';
  import Dashboard from './components/Dashboard.svelte';
  import ToastHost from './components/ToastHost.svelte';
  import NightMode from './components/NightMode.svelte';
  import AlertOverlay from './components/AlertOverlay.svelte';
  import { msUntil } from './lib/schedule';

  onMount(() => connection.start());

  // Server-driven look & language (hot-reloadable via config).
  $effect(() => {
    const cfg = connection.config;
    if (!cfg) return;
    i18n.locale = cfg.locale;
    document.documentElement.lang = cfg.locale;
    const root = document.documentElement;
    if (cfg.theme.mode === 'auto') root.removeAttribute('data-theme');
    else root.setAttribute('data-theme', cfg.theme.mode);
    if (cfg.theme.accent) root.style.setProperty('--accent', cfg.theme.accent);
    else root.style.removeProperty('--accent');
    // Background pictures: one per color scheme, picked by the --bg-image token (tokens.css).
    const bg = cfg.theme.background;
    for (const [name, url] of [['light', bg.light], ['dark', bg.dark]]) {
      if (url) root.style.setProperty(`--bg-image-${name}`, `url("${url}")`);
      else root.style.removeProperty(`--bg-image-${name}`);
    }
    if (bg.light || bg.dark) root.setAttribute('data-background', '');
    else root.removeAttribute('data-background');
    root.style.setProperty('--widget-opacity', String(cfg.theme.widgetOpacity));
    // Card look (theme.style); lite mode is always flat.
    if (cfg.theme.style !== 'flat' && !cfg.display.lite) root.setAttribute('data-style', cfg.theme.style);
    else root.removeAttribute('data-style');
    if (cfg.display.lite) root.setAttribute('data-lite', '');
    else root.removeAttribute('data-lite');
  });

  // Nightly soft reload: a safety net for a page that runs for weeks. It also
  // picks up a new frontend version. The extra second keeps a slightly early
  // timer from reloading twice.
  $effect(() => {
    const at = connection.config?.display.softReload;
    if (!at) return;
    const timer = window.setTimeout(() => location.reload(), msUntil(new Date(), at) + 1000);
    return () => window.clearTimeout(timer);
  });
</script>

{#if connection.config}
  <Dashboard config={connection.config} />
{:else}
  <div class="splash">{i18n.t(connection.status === 'offline' ? 'connection.offline' : 'app.noConfig')}</div>
{/if}

{#if connection.config?.display.night.enabled}
  <NightMode night={connection.config.display.night} />
{/if}

<ToastHost />

<AlertOverlay />

{#if connection.config && connection.status !== 'online'}
  <div class="banner" role="status">
    {i18n.t(connection.status === 'connecting' ? 'connection.connecting' : 'connection.offline')}
  </div>
{/if}

<style>
  .splash {
    height: 100%;
    display: grid;
    place-items: center;
    color: var(--muted);
  }
  .banner {
    position: fixed;
    top: var(--space-3);
    left: 50%;
    transform: translateX(-50%);
    padding: var(--space-2) var(--space-4);
    border-radius: 999px;
    background: var(--danger);
    color: #fff;
    font-size: var(--text-s);
    z-index: 10;
  }
</style>
