<script lang="ts">
  // Line icons for openHAB icon names. Unknown names get a neutral dot.
  let { name, size = 24 }: { name: string; size?: number } = $props();

  const icons: Record<string, string> = {
    bulb: 'M9 18h6M10 21h4M12 3a6 6 0 0 0-4 10.5c.7.7 1 1.5 1 2.5h6c0-1 .3-1.8 1-2.5A6 6 0 0 0 12 3z',
    power: 'M12 3v8M6.4 6.6a8 8 0 1 0 11.2 0',
    palette:
      'M12 3a9 9 0 1 0 0 18c1.2 0 2-.8 2-1.8 0-.6-.3-1-.6-1.4-.3-.4-.4-.8-.4-1.2 0-1 .8-1.6 1.8-1.6H17a4 4 0 0 0 4-4c0-4.4-4-8-9-8zM7.5 11.5h.01M10 7.5h.01M14.5 7.5h.01',
    sun: 'M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
    thermometer: 'M14 14.8V5a2 2 0 0 0-4 0v9.8a4 4 0 1 0 4 0z',
    drop: 'M12 3s6 6.2 6 10.5a6 6 0 0 1-12 0C6 9.2 12 3 12 3z',
    window: 'M4 3h16v18H4zM12 3v18M4 12h16',
    door: 'M6 21V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v17M4 21h16M14.5 12v.01',
    blinds: 'M4 4h16M5 8h14M5 12h14M5 16h14M12 16v4',
    battery: 'M3 8h15v8H3zM21 11v2M6 11v2',
    house: 'M3 11l9-8 9 8M5 10v10h14V10M10 20v-6h4v6',
    rain: 'M7 15a4 4 0 0 1 .5-8A5.5 5.5 0 0 1 18 8.5 3.5 3.5 0 0 1 17.5 15M9 18l-1 3M13 18l-1 3M17 18l-1 3',
    clock: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7v5l3 2',
    dot: 'M12 4a8 8 0 1 0 0 16 8 8 0 0 0 0-16zM12 11a1 1 0 1 0 0 2 1 1 0 0 0 0-2z',
  };

  // openHAB icon name -> our icon
  const alias: Record<string, string> = {
    switch: 'power',
    light: 'bulb',
    lightbulb: 'bulb',
    colorwheel: 'palette',
    colorpicker: 'palette',
    solarplant: 'sun',
    energy: 'sun',
    temperature: 'thermometer',
    humidity: 'drop',
    water: 'drop',
    screen: 'blinds',
    rollershutter: 'blinds',
    batterylevel: 'battery',
    time: 'clock',
    rain: 'rain',
  };

  const key = $derived(alias[name] ?? (name in icons ? name : 'dot'));
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d={icons[key]} />
</svg>

<style>
  svg {
    display: block;
    flex: none;
  }
</style>
