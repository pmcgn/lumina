// Date logic for the calendar widget. "Days" are calendar dates (YYYY-MM-DD) in
// the configured timezone, never instants, so they cannot drift across DST or
// between the server's and the tablet's timezone.

import type { Event as CalEvent } from './model.gen';

export type { CalEvent };

export interface DayGroup {
  key: string;
  events: CalEvent[];
}

export interface GridCell {
  key: string;
  day: number;
  inMonth: boolean;
  today: boolean;
}

const formatters = new Map<string, Intl.DateTimeFormat>();

/** The calendar date of an instant in the given timezone. */
export function dateKey(ms: number, tz: string): string {
  let f = formatters.get(tz);
  if (!f) {
    f = new Intl.DateTimeFormat('en-CA', { timeZone: tz, year: 'numeric', month: '2-digit', day: '2-digit' });
    formatters.set(tz, f);
  }
  return f.format(ms); // en-CA gives YYYY-MM-DD
}

function toUTC(key: string): number {
  const [y, m, d] = key.split('-').map(Number);
  return Date.UTC(y, m - 1, d);
}

export function addDays(key: string, n: number): string {
  return new Date(toUTC(key) + n * 86_400_000).toISOString().slice(0, 10);
}

/** Whole days from a to b (b - a). */
export function dayDiff(a: string, b: string): number {
  return Math.round((toUTC(b) - toUTC(a)) / 86_400_000);
}

/** The last calendar date an event touches (the end is exclusive). */
export function lastDayKey(e: CalEvent, tz: string): string {
  const startKey = dateKey(e.start * 1000, tz);
  const endKey = dateKey(Math.max(e.end - 1, e.start) * 1000, tz);
  return endKey < startKey ? startKey : endKey;
}

/**
 * Groups upcoming events by day. Events that are over are dropped (unless
 * showPast); events that started earlier but are still running (multi-day) are
 * shown under today.
 */
export function groupAgenda(events: CalEvent[], nowMs: number, tz: string, showPast = false): DayGroup[] {
  const today = dateKey(nowMs, tz);
  const groups = new Map<string, CalEvent[]>();
  for (const e of events) {
    if (!showPast && e.end * 1000 <= nowMs && e.end > e.start) continue;
    if (!showPast && e.end === e.start && e.start * 1000 < nowMs - 60_000) continue; // zero-length, already past
    const start = dateKey(e.start * 1000, tz);
    const key = !showPast && start < today ? today : start;
    const list = groups.get(key) ?? [];
    list.push(e);
    groups.set(key, list);
  }
  return [...groups.entries()]
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([key, list]) => ({
      key,
      events: list.sort((a, b) => Number(!!b.allDay) - Number(!!a.allDay) || a.start - b.start || a.cal - b.cal),
    }));
}

/** The first event that has not started yet, if it starts within `withinMs`. */
export function nextEvent(groups: DayGroup[], nowMs: number, withinMs = 3 * 3_600_000): CalEvent | undefined {
  for (const g of groups) {
    for (const e of g.events) {
      if (e.allDay) continue;
      if (e.start * 1000 > nowMs) return e.start * 1000 - nowMs <= withinMs ? e : undefined;
    }
  }
  return undefined;
}

/** Cells of the month overview: complete weeks covering the month of `todayKey`. */
export function monthGrid(monthFrom: string, todayKey: string): GridCell[] {
  const month = todayKey.slice(0, 7);
  const [y, m] = month.split('-').map(Number);
  const lastOfMonth = new Date(Date.UTC(y, m, 0)).toISOString().slice(0, 10);
  const rows = Math.floor(dayDiff(monthFrom, lastOfMonth) / 7) + 1;
  const cells: GridCell[] = [];
  for (let i = 0; i < rows * 7; i++) {
    const key = addDays(monthFrom, i);
    cells.push({ key, day: Number(key.slice(8)), inMonth: key.startsWith(month), today: key === todayKey });
  }
  return cells;
}
