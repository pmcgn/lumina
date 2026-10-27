import { describe, expect, it } from 'vitest';
import { formatParts, formatValue } from './units';

const NBSP = ' ';

describe('formatValue', () => {
  it('formats plain numbers with sensible automatic decimals', () => {
    expect(formatValue(3, '', undefined, 'en')).toBe('3');
    expect(formatValue(3.14159, 'none', undefined, 'en')).toBe('3.14');
    expect(formatValue(21.456, '', undefined, 'en')).toBe('21.5');
    expect(formatValue(1234.5, '', undefined, 'en')).toBe('1,235');
  });

  it('uses the locale for decimal and thousands separators', () => {
    expect(formatValue(21.5, 'celsius', undefined, 'de')).toBe(`21,5${NBSP}°C`);
    expect(formatValue(21.5, 'celsius', undefined, 'en')).toBe(`21.5${NBSP}°C`);
  });

  it('respects fixed decimals', () => {
    expect(formatValue(21.456, 'celsius', 0, 'en')).toBe(`21${NBSP}°C`);
    expect(formatValue(21, 'celsius', 2, 'en')).toBe(`21.00${NBSP}°C`);
  });

  it('scales watts to kW and MW', () => {
    expect(formatValue(850, 'watt', undefined, 'en')).toBe(`850${NBSP}W`);
    expect(formatValue(11583.56, 'watt', undefined, 'en')).toBe(`11.6${NBSP}kW`);
    expect(formatValue(2_500_000, 'watt', undefined, 'en')).toBe(`2.50${NBSP}MW`);
    expect(formatValue(-1500, 'watt', undefined, 'en')).toBe(`-1.50${NBSP}kW`); // discharging battery
  });

  it('handles percent in both forms', () => {
    expect(formatValue(83, 'percent', undefined, 'en')).toBe(`83${NBSP}%`);
    expect(formatValue(83, '%', undefined, 'en')).toBe(`83${NBSP}%`);
    expect(formatValue(0.835, 'percentunit', 1, 'en')).toBe(`83.5${NBSP}%`);
  });

  it('scales bytes (IEC) and durations', () => {
    expect(formatValue(512, 'bytes', undefined, 'en')).toBe(`512${NBSP}B`);
    expect(formatValue(1536, 'bytes', undefined, 'en')).toBe(`1.50${NBSP}KiB`);
    expect(formatValue(90, 's', undefined, 'en')).toBe(`1.50${NBSP}min`);
    expect(formatValue(7200, 's', undefined, 'en')).toBe(`2${NBSP}h`);
    expect(formatValue(1500, 'ms', undefined, 'en')).toBe(`1.50${NBSP}s`);
  });

  it('knows common household units', () => {
    expect(formatValue(12.3, 'velocitykmh', undefined, 'en')).toBe(`12.3${NBSP}km/h`);
    expect(formatValue(51.48, 'volt', undefined, 'en')).toBe(`51.5${NBSP}V`);
    expect(formatValue(1013, 'pressurehpa', undefined, 'en')).toBe(`1,013${NBSP}hPa`);
  });

  it('supports suffix units and falls back gracefully for unknown ones', () => {
    expect(formatValue(5, 'suffix: Stk', undefined, 'en')).toBe(`5${NBSP}Stk`);
    expect(formatValue(5, 'mph', undefined, 'en')).toBe(`5${NBSP}mph`); // short unknown id -> shown as is
    expect(formatValue(5, 'some_long_unknown_unit', undefined, 'en')).toBe('5'); // too odd to show
  });

  it('shows a dash for values that are not numbers', () => {
    expect(formatValue(NaN, 'watt')).toBe('–');
    expect(formatValue(Infinity, 'watt')).toBe('–');
  });
});

describe('formatParts', () => {
  it('separates the number from the unit (for big stat displays)', () => {
    expect(formatParts(11583.56, 'watt', undefined, 'en')).toEqual({ value: '11.6', unit: 'kW' });
    expect(formatParts(7, '', undefined, 'en')).toEqual({ value: '7', unit: '' });
  });
});
