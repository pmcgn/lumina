import { describe, expect, it } from 'vitest';
import { addDays, dateKey, dayDiff, groupAgenda, lastDayKey, monthGrid, nextEvent, type CalEvent } from './agenda';

const BERLIN = 'Europe/Berlin';
const sec = (iso: string) => Math.floor(new Date(iso).getTime() / 1000);
// Sat 17 Oct 2026, 12:00 Berlin (CEST, UTC+2)
const NOW = new Date('2026-10-17T10:00:00Z').getTime();

const timed = (title: string, startIso: string, endIso: string, cal = 0): CalEvent => ({ title, start: sec(startIso), end: sec(endIso), cal });
const allDay = (title: string, startKeyBerlin: string, endKeyBerlin: string, cal = 0): CalEvent => ({
  title,
  allDay: true,
  cal,
  // local midnight in Berlin as the backend sends it
  start: sec(`${startKeyBerlin}T00:00:00+02:00`.replace('+02:00', startKeyBerlin >= '2026-10-25' ? '+01:00' : '+02:00')),
  end: sec(`${endKeyBerlin}T00:00:00+02:00`.replace('+02:00', endKeyBerlin >= '2026-10-25' ? '+01:00' : '+02:00')),
});

describe('date keys', () => {
  it('uses the calendar date in the given timezone', () => {
    // 22:30 UTC on the 17th is already the 18th in Berlin (UTC+2) and Tokyo, still the 17th in New York
    const ms = new Date('2026-10-17T22:30:00Z').getTime();
    expect(dateKey(ms, BERLIN)).toBe('2026-10-18');
    expect(dateKey(ms, 'Asia/Tokyo')).toBe('2026-10-18');
    expect(dateKey(ms, 'America/New_York')).toBe('2026-10-17');
    expect(dateKey(ms, 'UTC')).toBe('2026-10-17');
  });

  it('adds days and measures differences across month, year and DST changes', () => {
    expect(addDays('2026-10-31', 1)).toBe('2026-11-01');
    expect(addDays('2026-12-31', 1)).toBe('2027-01-01');
    expect(addDays('2026-03-01', -1)).toBe('2026-02-28');
    expect(dayDiff('2026-10-24', '2026-10-26')).toBe(2); // DST ends on the 25th: a 25 h day
    expect(dayDiff('2026-10-26', '2026-10-24')).toBe(-2);
  });

  it('knows the last day an event touches (end is exclusive)', () => {
    // 22:00-00:00 Berlin: ends exactly at midnight, so it does not touch the 18th
    expect(lastDayKey(timed('x', '2026-10-17T20:00:00Z', '2026-10-17T22:00:00Z'), BERLIN)).toBe('2026-10-17');
    // one minute longer does
    expect(lastDayKey(timed('y', '2026-10-17T20:00:00Z', '2026-10-17T22:01:00Z'), BERLIN)).toBe('2026-10-18');
    expect(lastDayKey(allDay('trip', '2026-10-23', '2026-10-26'), BERLIN)).toBe('2026-10-25');
    expect(lastDayKey(allDay('one', '2026-10-20', '2026-10-21'), BERLIN)).toBe('2026-10-20');
  });
});

describe('groupAgenda', () => {
  it('groups by local day, all-day first, and drops events that are over', () => {
    const groups = groupAgenda(
      [
        timed('Over', '2026-10-17T06:00:00Z', '2026-10-17T07:00:00Z'), // 08:00-09:00 Berlin, finished
        timed('Running', '2026-10-17T09:00:00Z', '2026-10-17T11:00:00Z'), // 11:00-13:00 Berlin, now 12:00
        timed('Later', '2026-10-17T15:00:00Z', '2026-10-17T16:00:00Z'),
        allDay('Birthday', '2026-10-17', '2026-10-18'),
        timed('Tomorrow', '2026-10-18T07:00:00Z', '2026-10-18T08:00:00Z'),
      ],
      NOW,
      BERLIN,
    );
    expect(groups.map((g) => g.key)).toEqual(['2026-10-17', '2026-10-18']);
    expect(groups[0].events.map((e) => e.title)).toEqual(['Birthday', 'Running', 'Later']);
  });

  it('puts an event just after local midnight on the next day, not the UTC day', () => {
    // 22:30 UTC on the 17th = 00:30 on the 18th in Berlin
    const groups = groupAgenda([timed('Night owl', '2026-10-17T22:30:00Z', '2026-10-17T23:30:00Z')], NOW, BERLIN);
    expect(groups.map((g) => g.key)).toEqual(['2026-10-18']);
  });

  it('shows a running multi-day event under today', () => {
    const groups = groupAgenda([allDay('Holiday', '2026-10-15', '2026-10-20')], NOW, BERLIN);
    expect(groups.map((g) => g.key)).toEqual(['2026-10-17']);
  });

  it('keeps past events with showPast and anchors them at their own day', () => {
    const groups = groupAgenda([timed('Over', '2026-10-17T06:00:00Z', '2026-10-17T07:00:00Z'), allDay('Old', '2026-10-15', '2026-10-16')], NOW, BERLIN, true);
    expect(groups.map((g) => g.key)).toEqual(['2026-10-15', '2026-10-17']);
  });

  it('returns nothing when everything is over', () => {
    expect(groupAgenda([timed('Over', '2026-10-17T06:00:00Z', '2026-10-17T07:00:00Z')], NOW, BERLIN)).toEqual([]);
  });
});

describe('nextEvent', () => {
  const groups = groupAgenda(
    [timed('Running', '2026-10-17T09:00:00Z', '2026-10-17T11:00:00Z'), timed('Soon', '2026-10-17T11:00:00Z', '2026-10-17T12:00:00Z'), timed('Far', '2026-10-18T11:00:00Z', '2026-10-18T12:00:00Z')],
    NOW,
    BERLIN,
  );

  it('is the first event that has not started, if it is close', () => {
    expect(nextEvent(groups, NOW)?.title).toBe('Soon'); // in 1 h
  });

  it('is nothing when the next event is far away', () => {
    const far = groupAgenda([timed('Far', '2026-10-18T11:00:00Z', '2026-10-18T12:00:00Z')], NOW, BERLIN);
    expect(nextEvent(far, NOW)).toBeUndefined();
  });
});

describe('monthGrid', () => {
  it('covers the month with complete Monday-first weeks (October 2026: 5 rows)', () => {
    const cells = monthGrid('2026-09-28', '2026-10-17');
    expect(cells).toHaveLength(35);
    expect(cells[0]).toMatchObject({ key: '2026-09-28', day: 28, inMonth: false, today: false });
    expect(cells[3]).toMatchObject({ key: '2026-10-01', inMonth: true });
    expect(cells.filter((c) => c.today).map((c) => c.key)).toEqual(['2026-10-17']);
    expect(cells[34]).toMatchObject({ key: '2026-11-01', inMonth: false });
  });

  it('uses 6 rows when the month needs them (August 2026 starts on a Saturday)', () => {
    expect(monthGrid('2026-07-27', '2026-08-10')).toHaveLength(42);
  });

  it('uses 4 rows for a February that fits (Feb 2027 starts on a Monday)', () => {
    expect(monthGrid('2027-02-01', '2027-02-10')).toHaveLength(28);
  });

  it('supports Sunday-first grids', () => {
    const cells = monthGrid('2026-09-27', '2026-10-17');
    expect(cells[0].key).toBe('2026-09-27');
    expect(cells).toHaveLength(35);
  });
});
