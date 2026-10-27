// Pure helpers of the Grafana widget (kept apart from the component for testing).
import type { Data, Threshold } from './model.gen';

export const PALETTE = ['#3b82f6', '#f59e0b', '#10b981', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16'];

export const seriesColor = (color: string | undefined, index: number): string => color || PALETTE[index % PALETTE.length];

/**
 * The color of the highest threshold step at or below the value. The base step
 * has no value (null on the wire). Steps without a usable color are skipped;
 * `fallback` is used when nothing applies.
 */
export function thresholdColor(value: number, thresholds: Threshold[] | undefined, fallback: string): string {
  let color = '';
  for (const step of thresholds ?? []) {
    if ((step.value == null || value >= step.value) && step.color) color = step.color;
    else if (step.value != null && value < step.value) break;
  }
  return color || fallback;
}

/** True if the thresholds carry any information (more than a single base color). */
export const hasThresholds = (t: Threshold[] | undefined): boolean => (t?.length ?? 0) > 1;

/** The dial range: the panel's min/max, else sensible defaults for the unit, else from the thresholds. */
export function gaugeRange(d: Pick<Data, 'min' | 'max' | 'unit' | 'thresholds'>, value: number): [number, number] {
  let lo = d.min;
  let hi = d.max;
  if (lo == null) lo = d.unit === 'percent' || d.unit === '%' || d.unit === 'percentunit' || value >= 0 ? 0 : value * 1.25;
  if (hi == null) {
    if (d.unit === 'percent' || d.unit === '%') hi = 100;
    else if (d.unit === 'percentunit') hi = 1;
    else {
      const top = Math.max(value, ...(d.thresholds ?? []).map((t) => t.value ?? -Infinity));
      hi = top > lo ? top * 1.2 : lo + 1;
    }
  }
  return hi > lo ? [lo, hi] : [lo, lo + 1];
}

/** Position of a value on the dial, 0..1. */
export const fraction = (value: number, lo: number, hi: number): number => Math.min(1, Math.max(0, (value - lo) / (hi - lo)));

/** SVG path of a half-circle arc from fraction a to fraction b (0 = left end, 1 = right end). */
export function arcPath(cx: number, cy: number, r: number, a: number, b: number): string {
  const point = (f: number) => {
    const theta = Math.PI + f * Math.PI; // left -> over the top -> right
    return [cx + r * Math.cos(theta), cy + r * Math.sin(theta)].map((n) => n.toFixed(2));
  };
  const [x0, y0] = point(a);
  const [x1, y1] = point(b);
  return `M ${x0} ${y0} A ${r} ${r} 0 0 1 ${x1} ${y1}`;
}

/** How to label the time axis for a span: clock times for up to a day and a half, else weekday and day. */
export function timeFormat(spanMs: number, locale: string): (t: number) => string {
  if (spanMs <= 36 * 3_600_000) {
    const f = new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit', hour12: false });
    return (t) => f.format(t);
  }
  const f = new Intl.DateTimeFormat(locale, { weekday: 'short', day: 'numeric' });
  return (t) => f.format(t);
}

/** The last point of a series, if any. */
export const last = (points: { t: number; v: number }[]) => (points.length ? points[points.length - 1] : undefined);
