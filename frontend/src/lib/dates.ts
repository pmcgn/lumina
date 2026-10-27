const DAY = 86_400_000;

export type DueState = 'overdue' | 'today' | 'soon' | 'later';

export interface DueInfo {
  text: string;
  state: DueState;
}

function startOfDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/**
 * Describes a due date relative to `now` (e.g. "gestern, 14:00", "Morgen",
 * "Mo., 20.10."). Dates without a time of day (00:00) count as overdue only
 * after their day has passed, not on the day itself.
 */
export function dueInfo(dueSec: number, now: Date, locale: string): DueInfo {
  const due = new Date(dueSec * 1000);
  const days = Math.round((startOfDay(due) - startOfDay(now)) / DAY);
  const hasTime = due.getHours() !== 0 || due.getMinutes() !== 0;
  const time = hasTime ? due.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' }) : '';

  const overdue = hasTime ? due.getTime() < now.getTime() : days < 0;
  const state: DueState = overdue ? 'overdue' : days === 0 ? 'today' : days <= 2 ? 'soon' : 'later';

  let day: string;
  if (days >= -30 && days <= 1) {
    day = capitalize(new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(days, 'day'));
  } else if (days > 1 && days <= 6) {
    day = new Intl.DateTimeFormat(locale, { weekday: 'long' }).format(due);
  } else {
    day = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short', year: days < -300 || days > 300 ? 'numeric' : undefined }).format(due);
  }
  return { text: time ? `${day}, ${time}` : day, state };
}
