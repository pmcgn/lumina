/** Time-of-day helpers for the night window and the nightly reload (device local time). */

/** "23:30" -> minutes since midnight. */
export function parseHHMM(s: string): number {
  const [h, m] = s.split(':').map(Number);
  return h * 60 + m;
}

export function minutesOfDay(d: Date): number {
  return d.getHours() * 60 + d.getMinutes();
}

/** Is `now` inside [from, to)? The window may span midnight (e.g. 22:00-06:00). */
export function inWindow(now: Date, from: string, to: string): boolean {
  const n = minutesOfDay(now);
  const f = parseHHMM(from);
  const t = parseHHMM(to);
  return f < t ? n >= f && n < t : n >= f || n < t;
}

/** Milliseconds until the next occurrence of HH:MM; exactly now counts as tomorrow. */
export function msUntil(now: Date, hhmm: string): number {
  const [h, m] = hhmm.split(':').map(Number);
  const next = new Date(now);
  next.setHours(h, m, 0, 0);
  if (next.getTime() <= now.getTime()) next.setDate(next.getDate() + 1); // setDate handles DST changes
  return next.getTime() - now.getTime();
}
