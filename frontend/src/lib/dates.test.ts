import { describe, expect, it } from 'vitest';
import { dueInfo } from './dates';

const now = new Date(2026, 9, 17, 12, 0, 0); // Sat 17 Oct 2026, 12:00 local
const ts = (d: Date) => Math.floor(d.getTime() / 1000);
const local = (day: number, h = 0, m = 0) => new Date(2026, 9, day, h, m, 0);

describe('dueInfo state', () => {
  it('a timed task in the past is overdue', () => {
    expect(dueInfo(ts(local(17, 10, 30)), now, 'de').state).toBe('overdue');
    expect(dueInfo(ts(local(14, 8, 0)), now, 'de').state).toBe('overdue');
  });

  it('a timed task later today is "today"', () => {
    expect(dueInfo(ts(local(17, 18, 0)), now, 'de').state).toBe('today');
  });

  it('a date-only task (00:00) is not overdue on its own day, but is afterwards', () => {
    expect(dueInfo(ts(local(17)), now, 'de').state).toBe('today');
    expect(dueInfo(ts(local(16)), now, 'de').state).toBe('overdue');
  });

  it('tomorrow and the day after are "soon", later is "later"', () => {
    expect(dueInfo(ts(local(18, 9)), now, 'de').state).toBe('soon');
    expect(dueInfo(ts(local(19, 9)), now, 'de').state).toBe('soon');
    expect(dueInfo(ts(local(25, 9)), now, 'de').state).toBe('later');
  });
});

describe('dueInfo text', () => {
  it('shows the time only when the task has one', () => {
    expect(dueInfo(ts(local(18, 14, 5)), now, 'en').text).toMatch(/14:05|2:05/);
    expect(dueInfo(ts(local(18)), now, 'en').text).not.toMatch(/:/);
  });

  it('uses relative words near today', () => {
    expect(dueInfo(ts(local(18, 9)), now, 'en').text).toMatch(/^Tomorrow/);
    expect(dueInfo(ts(local(16, 9)), now, 'en').text).toMatch(/^Yesterday/);
    expect(dueInfo(ts(local(17, 18)), now, 'en').text).toMatch(/^Today/);
  });
});
