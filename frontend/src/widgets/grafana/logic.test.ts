import { describe, expect, it } from 'vitest';
import { arcPath, fraction, gaugeRange, hasThresholds, seriesColor, thresholdColor, timeFormat, PALETTE } from './logic';

describe('thresholdColor', () => {
  const steps = [
    { color: '#green' }, // base step: no value (null on the wire)
    { value: 3000, color: '#orange' },
    { value: 4000, color: '#red' },
  ];

  it('picks the highest step at or below the value', () => {
    expect(thresholdColor(0, steps, '#fb')).toBe('#green');
    expect(thresholdColor(2999.9, steps, '#fb')).toBe('#green');
    expect(thresholdColor(3000, steps, '#fb')).toBe('#orange');
    expect(thresholdColor(9999, steps, '#fb')).toBe('#red');
  });

  it('treats a null base value like a missing one and works below zero', () => {
    expect(thresholdColor(-500, [{ value: null as unknown as undefined, color: '#base' }, { value: 0, color: '#pos' }], '#fb')).toBe('#base');
  });

  it('uses the last of several steps with the same value (a Grafana quirk of battery panels)', () => {
    expect(thresholdColor(5, [{ value: 0, color: '#blue' }, { value: 0, color: '#darkgreen' }], '#fb')).toBe('#darkgreen');
  });

  it('skips steps without a usable color and falls back', () => {
    expect(thresholdColor(10, [{ color: '' }], '#fb')).toBe('#fb');
    expect(thresholdColor(10, undefined, '#fb')).toBe('#fb');
    expect(thresholdColor(5000, [{ color: '#a' }, { value: 4000, color: '' }], '#fb')).toBe('#a');
  });

  it('knows whether thresholds say anything', () => {
    expect(hasThresholds(undefined)).toBe(false);
    expect(hasThresholds([{ color: '#a' }])).toBe(false);
    expect(hasThresholds([{ color: '#a' }, { value: 5, color: '#b' }])).toBe(true);
  });
});

describe('gaugeRange', () => {
  it('uses the panel range when given', () => {
    expect(gaugeRange({ min: 42, max: 58 }, 51)).toEqual([42, 58]);
    expect(gaugeRange({ min: -3000, max: 3000 }, -800)).toEqual([-3000, 3000]);
  });

  it('defaults percent units to 0..100 and 0..1', () => {
    expect(gaugeRange({ unit: 'percent' }, 83)).toEqual([0, 100]);
    expect(gaugeRange({ unit: '%' }, 83)).toEqual([0, 100]);
    expect(gaugeRange({ unit: 'percentunit' }, 0.4)).toEqual([0, 1]);
  });

  it('derives a range from the value and thresholds otherwise', () => {
    expect(gaugeRange({ thresholds: [{ color: 'a' }, { value: 80, color: 'b' }] }, 20)).toEqual([0, 96]);
    expect(gaugeRange({}, 50)).toEqual([0, 60]);
  });

  it('never returns an empty range', () => {
    expect(gaugeRange({ min: 5, max: 5 }, 5)).toEqual([5, 6]);
    expect(gaugeRange({}, 0)[1]).toBeGreaterThan(0);
  });
});

describe('fraction and arcPath', () => {
  it('maps values onto 0..1 and clamps', () => {
    expect(fraction(50, 0, 100)).toBe(0.5);
    expect(fraction(-10, 0, 100)).toBe(0);
    expect(fraction(500, 0, 100)).toBe(1);
    expect(fraction(0, -3000, 3000)).toBe(0.5);
  });

  it('draws the dial from the left end over the top to the right end', () => {
    // r=80 around (100,100): left end (20,100), top (100,20), right end (180,100)
    expect(arcPath(100, 100, 80, 0, 0.5)).toBe('M 20.00 100.00 A 80 80 0 0 1 100.00 20.00');
    expect(arcPath(100, 100, 80, 0.5, 1)).toBe('M 100.00 20.00 A 80 80 0 0 1 180.00 100.00');
  });
});

describe('timeFormat', () => {
  const noon = new Date(2026, 9, 18, 14, 5).getTime();

  it('shows clock times for short spans', () => {
    expect(timeFormat(6 * 3_600_000, 'en')(noon)).toBe('14:05');
  });

  it('shows weekday and day for long spans', () => {
    expect(timeFormat(7 * 24 * 3_600_000, 'en')(noon)).toMatch(/Sun.*18|18.*Sun/);
  });
});

describe('seriesColor', () => {
  it('prefers the panel color and cycles through the palette', () => {
    expect(seriesColor('#123456', 0)).toBe('#123456');
    expect(seriesColor(undefined, 1)).toBe(PALETTE[1]);
    expect(seriesColor('', PALETTE.length)).toBe(PALETTE[0]);
  });
});
