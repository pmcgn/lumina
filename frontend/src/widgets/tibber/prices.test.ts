import { describe, expect, it } from 'vitest';
import { currentSlot, levelOf, priceUnit, stats, thresholds, upcoming } from './prices';
import type { Slot } from './types';

const T0 = Date.UTC(2026, 10, 8, 0, 0) / 1000;
const slots = (prices: number[]): Slot[] => prices.map((price, i) => ({ t: T0 + i * 900, price }));

describe('currentSlot', () => {
  const s = slots([0.1, 0.2, 0.3]);
  it('finds the slot containing now, including its first second', () => {
    expect(currentSlot(s, (T0 + 900) * 1000, 900)?.price).toBe(0.2);
    expect(currentSlot(s, (T0 + 1799) * 1000 + 999, 900)?.price).toBe(0.2);
  });
  it('is undefined outside the data', () => {
    expect(currentSlot(s, (T0 - 1) * 1000, 900)).toBeUndefined();
    expect(currentSlot(s, (T0 + 2700) * 1000, 900)).toBeUndefined();
  });
});

describe('levels', () => {
  const s = slots([0.1, 0.1, 0.2, 0.2, 0.3, 0.3]);
  const th = thresholds(s);
  it('splits into thirds', () => {
    expect(levelOf(0.1, th)).toBe('cheap');
    expect(levelOf(0.2, th)).toBe('normal');
    expect(levelOf(0.3, th)).toBe('expensive');
  });
  it('treats flat prices as normal', () => {
    expect(levelOf(0.25, thresholds(slots([0.25, 0.25, 0.25])))).toBe('normal');
  });
  it('copes with no data', () => {
    expect(thresholds([])).toEqual({ low: 0, high: 0 });
  });
});

describe('stats / upcoming', () => {
  const s = slots([0.3, 0.1, 0.2]);
  it('finds min, max and average', () => {
    const st = stats(s)!;
    expect(st.min.price).toBe(0.1);
    expect(st.max.price).toBe(0.3);
    expect(st.avg).toBeCloseTo(0.2);
    expect(stats([])).toBeUndefined();
  });
  it('keeps the running slot and the following ones', () => {
    expect(upcoming(s, (T0 + 1000) * 1000, 900).map((x) => x.price)).toEqual([0.1, 0.2]);
  });
});

describe('priceUnit', () => {
  it('uses the subunit where there is one', () => {
    expect(priceUnit('EUR')).toEqual({ label: 'ct/kWh', factor: 100, decimals: 1 });
    expect(priceUnit('NOK').label).toBe('øre/kWh');
  });
  it('falls back to the currency', () => {
    expect(priceUnit('XYZ')).toEqual({ label: 'XYZ/kWh', factor: 1, decimals: 3 });
  });
});
