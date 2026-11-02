<script lang="ts">
  // Battery icon drawn natively, in the style of WeatherIcon (flat shapes, colors from tokens).
  // `percent` is the charge, 0..100; `text` is printed inside the icon when given.
  // variant "classic": solid icon filled with `color`. "minimal": thin outline, translucent accent fill.
  let {
    percent,
    orientation = 'horizontal',
    variant = 'classic',
    color = 'var(--ok)',
    text,
    length = 120,
  }: {
    percent: number;
    orientation?: 'horizontal' | 'vertical';
    variant?: 'classic' | 'minimal';
    color?: string;
    text?: string;
    length?: number;
  } = $props();

  const uid = $props.id();

  // Geometry in viewBox units. Horizontal is 104x56, vertical the same turned upright (60x104).
  const vertical = $derived(orientation === 'vertical');
  const minimal = $derived(variant === 'minimal');
  const vbW = $derived(vertical ? 60 : 104);
  const vbH = $derived(vertical ? 104 : 56);
  const body = $derived(
    minimal
      ? vertical ? { x: 5, y: 15, w: 50, h: 84 } : { x: 3, y: 5, w: 86, h: 46 }
      : vertical ? { x: 4, y: 14, w: 52, h: 88 } : { x: 2, y: 4, w: 88, h: 48 },
  );
  const nub = $derived(vertical ? { x: 20, y: 2, w: 20, h: 18 } : { x: 84, y: 18, w: 18, h: 20 });
  const cavity = $derived(
    minimal
      ? vertical ? { x: 11, y: 21, w: 38, h: 72 } : { x: 9, y: 11, w: 74, h: 34 }
      : vertical ? { x: 9, y: 19, w: 42, h: 78 } : { x: 7, y: 9, w: 78, h: 38 },
  );
  // minimal: the terminal is an open line instead of a solid block
  const cap = $derived(
    vertical ? 'M 22 15 V12 a3 3 0 0 1 3 -3 h10 a3 3 0 0 1 3 3 V15' : 'M 89 21 h3 a3 3 0 0 1 3 3 v8 a3 3 0 0 1 -3 3 h-3',
  );

  // the fill: a sliver stays visible for any charge above zero
  const frac = $derived(Math.min(1, Math.max(0, percent / 100)));
  const span = $derived((vertical ? cavity.h : cavity.w) * frac);
  const filled = $derived(frac > 0 ? Math.max(span, minimal ? 4 : 5) : 0);
  const fillRect = $derived(
    vertical
      ? { x: cavity.x, y: cavity.y + cavity.h - filled, w: cavity.w, h: filled }
      : { x: cavity.x, y: cavity.y, w: filled, h: cavity.h },
  );

  // text size: as large as fits the cavity, shrinks for long labels
  const fontSize = $derived(
    text
      ? minimal
        ? Math.min(cavity.h * 0.58, (cavity.w - 6) / (Math.max(text.length, 2) * 0.55))
        : Math.min(cavity.h * 0.62, (cavity.w - 6) / (Math.max(text.length, 2) * 0.62))
      : 0,
  );
  const cx = $derived(cavity.x + cavity.w / 2);
  const cy = $derived(cavity.y + cavity.h / 2);

  const px = $derived(length / Math.max(vbW, vbH));
</script>

<svg width={vbW * px} height={vbH * px} viewBox="0 0 {vbW} {vbH}" role="img" aria-label="{Math.round(percent)} %">
  <defs>
    <clipPath id="cav-{uid}"><rect x={cavity.x} y={cavity.y} width={cavity.w} height={cavity.h} rx={minimal ? 6 : 7} /></clipPath>
    <clipPath id="fil-{uid}"><rect x={fillRect.x} y={fillRect.y} width={fillRect.w} height={fillRect.h} /></clipPath>
  </defs>

  {#if minimal}
    <g clip-path="url(#cav-{uid})">
      <rect x={fillRect.x} y={fillRect.y} width={fillRect.w} height={fillRect.h} fill="var(--accent)" opacity="0.38" />
    </g>
    <rect x={body.x} y={body.y} width={body.w} height={body.h} rx="11" class="line" />
    <path d={cap} class="line" stroke-linecap="round" />
    {#if text}
      <text x={cx} y={cy} dy="0.36em" text-anchor="middle" font-size={fontSize} class="thin">{text}</text>
    {/if}
  {:else}
    <rect x={nub.x} y={nub.y} width={nub.w} height={nub.h} rx="5" fill="var(--icon-battery)" />
    <rect x={body.x} y={body.y} width={body.w} height={body.h} rx="12" fill="var(--icon-battery)" />
    <rect x={cavity.x} y={cavity.y} width={cavity.w} height={cavity.h} rx="7" fill="var(--icon-battery-empty)" />
    <g clip-path="url(#cav-{uid})">
      <rect x={fillRect.x} y={fillRect.y} width={fillRect.w} height={fillRect.h} fill={color} />
    </g>

    {#if text}
      <text x={cx} y={cy} dy="0.36em" text-anchor="middle" font-size={fontSize} font-weight="600" fill="var(--text)">{text}</text>
      <g clip-path="url(#fil-{uid})">
        <text x={cx} y={cy} dy="0.36em" text-anchor="middle" font-size={fontSize} font-weight="600" fill="#fff">{text}</text>
      </g>
    {/if}
  {/if}
</svg>

<style>
  svg {
    display: block;
    flex: none;
    overflow: visible;
  }
  text {
    font-family: inherit;
    letter-spacing: -0.01em;
  }
  .line {
    fill: none;
    stroke: var(--muted);
    stroke-width: 2.2;
  }
  .thin {
    font-weight: 300;
    fill: var(--text);
  }
</style>
