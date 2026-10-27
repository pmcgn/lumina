<script lang="ts">
  // Weather icons drawn natively (no external assets). Colors come from tokens.
  import type { Condition } from '../widgets/weather/model.gen';

  let { condition, isDay = true, size = 48 }: { condition: Condition; isDay?: boolean; size?: number } = $props();

  const uid = $props.id();
  const cloudPath = 'M20 48h26a10 10 0 0 0 1.5-19.9A14 14 0 0 0 20.6 31 8.6 8.6 0 0 0 20 48z';

  // Which extras are drawn for a condition.
  const withSun = $derived(condition === 'partly-cloudy' || condition === 'showers' || condition === 'snow-showers');
  const hasCloud = $derived(condition !== 'clear');
  const precip = $derived(
    condition === 'drizzle' || condition === 'rain' || condition === 'showers' || condition === 'freezing-rain'
      ? 'rain'
      : condition === 'snow' || condition === 'snow-showers'
        ? 'snow'
        : condition === 'thunderstorm'
          ? 'bolt'
          : '',
  );
</script>

<svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true">
  <defs>
    <mask id="moon-{uid}">
      <rect width="64" height="64" fill="#fff" />
      <circle cx="40" cy="24" r="11" fill="#000" />
    </mask>
  </defs>

  <!-- celestial body -->
  {#if condition === 'clear' || withSun}
    {@const s = condition === 'clear' ? 1 : 0.62}
    {@const ox = condition === 'clear' ? 0 : -6}
    {@const oy = condition === 'clear' ? 0 : -8}
    <g transform="translate({ox + (1 - s) * 32},{oy + (1 - s) * 32}) scale({s})">
      {#if isDay}
        <circle cx="32" cy="32" r="11" fill="var(--icon-sun)" />
        {#each [0, 45, 90, 135, 180, 225, 270, 315] as a (a)}
          <line x1="32" y1="12" x2="32" y2="7" stroke="var(--icon-sun)" stroke-width="3.5" stroke-linecap="round" transform="rotate({a} 32 32)" />
        {/each}
      {:else}
        <circle cx="32" cy="32" r="15" fill="var(--icon-moon)" mask="url(#moon-{uid})" />
      {/if}
    </g>
  {/if}

  <!-- clouds -->
  {#if hasCloud && condition !== 'fog'}
    {#if condition === 'cloudy'}
      <path d={cloudPath} fill="var(--icon-cloud-back)" transform="translate(-9,-9) scale(0.85)" />
    {/if}
    <path d={cloudPath} fill={condition === 'thunderstorm' ? 'var(--icon-cloud-dark)' : 'var(--icon-cloud)'} transform={precip ? 'translate(0,-6)' : ''} />
  {/if}

  <!-- precipitation -->
  {#if condition === 'drizzle'}
    {#each [24, 34, 44] as x (x)}
      <line x1={x} y1="48" x2={x - 2} y2="54" stroke="var(--icon-rain)" stroke-width="2.6" stroke-linecap="round" />
    {/each}
  {:else if precip === 'rain'}
    {#each [22, 32, 42] as x (x)}
      <line x1={x + 2} y1="46" x2={x - 2} y2="55" stroke={condition === 'freezing-rain' && x === 32 ? 'var(--icon-snow)' : 'var(--icon-rain)'} stroke-width="3" stroke-linecap="round" />
    {/each}
  {:else if precip === 'snow'}
    {#each [22, 32, 42] as x, i (x)}
      <circle cx={x} cy={i === 1 ? 54 : 50} r="2.8" fill="var(--icon-snow)" />
    {/each}
  {:else if precip === 'bolt'}
    <path d="M33 38 24 52h7l-3 10 11-15h-7l4-9z" fill="var(--icon-sun)" />
  {/if}

  <!-- fog -->
  {#if condition === 'fog'}
    <path d={cloudPath} fill="var(--icon-cloud-back)" transform="translate(0,-8)" />
    {#each [46, 53, 60] as y, i (y)}
      <line x1={14 + i * 3} y1={y - 2} x2={50 - i * 3} y2={y - 2} stroke="var(--icon-cloud)" stroke-width="3" stroke-linecap="round" />
    {/each}
  {/if}
</svg>

<style>
  svg {
    display: block;
    flex: none;
  }
</style>
