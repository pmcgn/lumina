import type { Slot } from './types';

export type Level = 'cheap' | 'normal' | 'expensive';

/** The slot containing `nowMs`, or undefined when the data does not cover it. */
export function currentSlot(slots: Slot[], nowMs: number, stepSec: number): Slot | undefined {
  const s = Math.floor(nowMs / 1000);
  return slots.find((x) => x.t <= s && s < x.t + stepSec);
}

/** Price thresholds: below `low` is cheap, above `high` expensive (the lowest / highest third of the slots; ties stay normal). */
export function thresholds(slots: Slot[]): { low: number; high: number } {
  if (!slots.length) return { low: 0, high: 0 };
  const sorted = slots.map((s) => s.price).sort((a, b) => a - b);
  const n = sorted.length;
  return { low: sorted[Math.floor(n / 3)], high: sorted[Math.ceil((2 * n) / 3) - 1] };
}

export function levelOf(price: number, th: { low: number; high: number }): Level {
  if (price < th.low) return 'cheap';
  if (price > th.high) return 'expensive';
  return 'normal';
}

export function stats(slots: Slot[]): { min: Slot; max: Slot; avg: number } | undefined {
  if (!slots.length) return undefined;
  let min = slots[0];
  let max = slots[0];
  let sum = 0;
  for (const s of slots) {
    if (s.price < min.price) min = s;
    if (s.price > max.price) max = s;
    sum += s.price;
  }
  return { min, max, avg: sum / slots.length };
}

/** Slots from `fromMs` on (the remaining prices of a day, for example). */
export function upcoming(slots: Slot[], fromMs: number, stepSec: number): Slot[] {
  const s = Math.floor(fromMs / 1000);
  return slots.filter((x) => x.t + stepSec > s);
}

const SUBUNIT: Record<string, string> = { EUR: 'ct', NOK: 'øre', SEK: 'öre', DKK: 'øre', CHF: 'Rp', GBP: 'p' };

/** Display unit: the subunit (ct, øre) with its factor, or the currency itself for unknown ones. */
export function priceUnit(currency: string): { label: string; factor: number; decimals: number } {
  const sub = SUBUNIT[currency];
  return sub ? { label: `${sub}/kWh`, factor: 100, decimals: 1 } : { label: `${currency}/kWh`, factor: 1, decimals: 3 };
}
