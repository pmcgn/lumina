import { describe, expect, it } from 'vitest';
import { inWindow, msUntil, parseHHMM } from './schedule';

const at = (h: number, m = 0, s = 0) => new Date(2026, 9, 17, h, m, s);

describe('inWindow', () => {
  it('handles a window that spans midnight (22:00-06:00)', () => {
    for (const [h, m, want] of [
      [21, 59, false],
      [22, 0, true],
      [23, 59, true],
      [0, 0, true],
      [3, 30, true],
      [5, 59, true],
      [6, 0, false], // end is exclusive
      [12, 0, false],
    ] as [number, number, boolean][]) {
      expect(inWindow(at(h, m), '22:00', '06:00'), `${h}:${m}`).toBe(want);
    }
  });

  it('handles a window within one day (13:00-15:30)', () => {
    expect(inWindow(at(12, 59), '13:00', '15:30')).toBe(false);
    expect(inWindow(at(13, 0), '13:00', '15:30')).toBe(true);
    expect(inWindow(at(15, 29), '13:00', '15:30')).toBe(true);
    expect(inWindow(at(15, 30), '13:00', '15:30')).toBe(false);
  });

  it('ignores seconds', () => {
    expect(inWindow(at(21, 59, 59), '22:00', '06:00')).toBe(false);
    expect(inWindow(at(22, 0, 0), '22:00', '06:00')).toBe(true);
  });
});

describe('msUntil', () => {
  it('returns the time until later today', () => {
    expect(msUntil(at(3, 0), '03:30')).toBe(30 * 60_000);
  });

  it('rolls over to tomorrow when the time has passed', () => {
    expect(msUntil(at(4, 0), '03:30')).toBe(23.5 * 3_600_000);
  });

  it('treats exactly now as tomorrow (no reload loop)', () => {
    expect(msUntil(at(3, 30, 0), '03:30')).toBe(24 * 3_600_000);
  });

  it('is always positive and at most a day', () => {
    for (let h = 0; h < 24; h++) {
      const ms = msUntil(at(h, 17, 42), '03:30');
      expect(ms).toBeGreaterThan(0);
      expect(ms).toBeLessThanOrEqual(25 * 3_600_000); // 25 h only on a DST change day
    }
  });
});

describe('parseHHMM', () => {
  it('converts to minutes', () => {
    expect(parseHHMM('00:00')).toBe(0);
    expect(parseHHMM('03:30')).toBe(210);
    expect(parseHHMM('23:59')).toBe(1439);
  });
});
