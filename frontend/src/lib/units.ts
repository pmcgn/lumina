// Formats values with Grafana unit ids (the "unit" of a panel), e.g. watt ->
// "11,5 kW". Only the common units are known; others fall back to a plain suffix.

export interface Parts {
  value: string; // formatted number
  unit: string; // e.g. "kW", "%", "" for none
}

const NBSP = ' ';

/** Fixed decimals if given, else a sensible number depending on magnitude. */
function digits(v: number, decimals?: number): number {
  if (decimals !== undefined) return decimals;
  const a = Math.abs(v);
  if (Number.isInteger(v) || a >= 100) return 0;
  return a >= 10 ? 1 : 2;
}

function num(v: number, decimals: number | undefined, locale: string): string {
  const d = digits(v, decimals);
  return new Intl.NumberFormat(locale, { minimumFractionDigits: d, maximumFractionDigits: d }).format(v);
}

/** Scales by thousands through a list of unit steps, e.g. W -> kW -> MW. */
function scale(v: number, steps: string[], base = 1000): { v: number; unit: string } {
  let i = 0;
  let x = v;
  while (Math.abs(x) >= base && i < steps.length - 1) {
    x /= base;
    i++;
  }
  return { v: x, unit: steps[i] };
}

const fixedSuffix: Record<string, string> = {
  percent: '%',
  '%': '%',
  humidity: '%',
  celsius: '°C',
  fahrenheit: '°F',
  kelvin: 'K',
  volt: 'V',
  amp: 'A',
  hertz: 'Hz',
  pressurehpa: 'hPa',
  pressurebar: 'bar',
  velocitykmh: 'km/h',
  velocityms: 'm/s',
  lengthm: 'm',
  lengthmm: 'mm',
  lengthkm: 'km',
  kwatt: 'kW',
  kwatth: 'kWh',
  currencyEUR: '€',
  currencyUSD: '$',
  currencyGBP: '£',
};

export function formatParts(v: number, unit = '', decimals?: number, locale = 'de'): Parts {
  if (!Number.isFinite(v)) return { value: '–', unit: '' };

  if (unit === '' || unit === 'none' || unit === 'short') return { value: num(v, decimals, locale), unit: '' };
  if (unit === 'percentunit') return { value: num(v * 100, decimals, locale), unit: '%' };

  switch (unit) {
    case 'watt': {
      const s = scale(v, ['W', 'kW', 'MW']);
      return { value: num(s.v, decimals, locale), unit: s.unit };
    }
    case 'watth': {
      const s = scale(v, ['Wh', 'kWh', 'MWh']);
      return { value: num(s.v, decimals, locale), unit: s.unit };
    }
    case 'bytes': {
      const s = scale(v, ['B', 'KiB', 'MiB', 'GiB', 'TiB'], 1024);
      return { value: num(s.v, decimals ?? (s.unit === 'B' ? 0 : undefined), locale), unit: s.unit };
    }
    case 'decbytes': {
      const s = scale(v, ['B', 'kB', 'MB', 'GB', 'TB']);
      return { value: num(s.v, decimals ?? (s.unit === 'B' ? 0 : undefined), locale), unit: s.unit };
    }
    case 'ms':
      return Math.abs(v) >= 1000 ? { value: num(v / 1000, decimals, locale), unit: 's' } : { value: num(v, decimals, locale), unit: 'ms' };
    case 's': {
      const a = Math.abs(v);
      if (a >= 3600) return { value: num(v / 3600, decimals, locale), unit: 'h' };
      if (a >= 60) return { value: num(v / 60, decimals, locale), unit: 'min' };
      return { value: num(v, decimals, locale), unit: 's' };
    }
    case 'm':
      return Math.abs(v) >= 60 ? { value: num(v / 60, decimals, locale), unit: 'h' } : { value: num(v, decimals, locale), unit: 'min' };
    case 'h':
      return Math.abs(v) >= 24 ? { value: num(v / 24, decimals, locale), unit: 'd' } : { value: num(v, decimals, locale), unit: 'h' };
  }
  if (unit in fixedSuffix) return { value: num(v, decimals, locale), unit: fixedSuffix[unit] };
  if (unit.startsWith('suffix:')) return { value: num(v, decimals, locale), unit: unit.slice(7).trim() };

  // Unknown unit id: show it as a suffix if it looks like a short symbol, otherwise omit it.
  return { value: num(v, decimals, locale), unit: /^[^\s]{1,5}$/.test(unit) ? unit : '' };
}

export function formatValue(v: number, unit = '', decimals?: number, locale = 'de'): string {
  const p = formatParts(v, unit, decimals, locale);
  return p.unit ? `${p.value}${NBSP}${p.unit}` : p.value;
}
