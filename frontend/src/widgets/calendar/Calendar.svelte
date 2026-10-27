<script lang="ts">
  import { i18n } from '../../lib/i18n.svelte';
  import type { WidgetProps } from '../../lib/registry';
  import { addDays, dateKey, dayDiff, groupAgenda, lastDayKey, monthGrid, nextEvent } from './agenda';
  import type { CalendarData, CalendarSettings } from './types';

  let { data, settings, size }: WidgetProps<CalendarData, CalendarSettings> = $props();
  const t = i18n.t;
  const tz = $derived(data.timezone);
  const locale = $derived(i18n.locale);

  // Tick so running/over events and the day change update without a server poll.
  let now = $state(Date.now());
  $effect(() => {
    const timer = window.setInterval(() => (now = Date.now()), 30_000);
    return () => window.clearInterval(timer);
  });

  const todayKey = $derived(dateKey(now, tz));
  const groups = $derived(groupAgenda(data.events, now, tz, settings.showPast ?? false));
  const next = $derived(nextEvent(groups, now));

  // Layout follows the card's size, not the screen.
  const compact = $derived(size.height < 200);
  const showMonth = $derived(settings.showMonth ?? (size.width >= 600 && size.height >= 330));

  const timeFmt = $derived(new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit', hour12: false, timeZone: tz }));
  const dayFmt = $derived(new Intl.DateTimeFormat(locale, { weekday: 'long', day: 'numeric', month: 'long', timeZone: tz }));
  const weekdayFmt = $derived(new Intl.DateTimeFormat(locale, { weekday: 'short', timeZone: tz }));
  const shortDayFmt = $derived(new Intl.DateTimeFormat(locale, { weekday: 'short', day: 'numeric', month: 'numeric', timeZone: tz }));

  function dayTitle(key: string): string {
    const diff = dayDiff(todayKey, key);
    if (diff === 0) return t('calendar.today');
    if (diff === 1) return t('calendar.tomorrow');
    const [y, m, d] = key.split('-').map(Number);
    return new Intl.DateTimeFormat(locale, { weekday: 'long', day: 'numeric', month: 'long', timeZone: 'UTC' }).format(Date.UTC(y, m - 1, d));
  }

  function when(e: (typeof data.events)[number]): { main: string; sub: string } {
    if (e.allDay) {
      const last = lastDayKey(e, tz);
      const first = dateKey(e.start * 1000, tz);
      return { main: t('calendar.allDay'), sub: last > first ? t('calendar.until', { when: shortDayFmt.format(e.end * 1000 - 1) }) : '' };
    }
    const start = timeFmt.format(e.start * 1000);
    const sameDay = dateKey(e.start * 1000, tz) === dateKey(Math.max(e.end - 1, e.start) * 1000, tz);
    if (e.end <= e.start) return { main: start, sub: '' };
    // Same day: "– 15:30". Crossing midnight: "– Mo. 00:10" (weekday keeps it short).
    return { main: start, sub: `– ${sameDay ? '' : weekdayFmt.format(e.end * 1000) + ' '}${timeFmt.format(e.end * 1000)}` };
  }

  const running = (e: { start: number; end: number; allDay?: boolean }) => !e.allDay && e.start * 1000 <= now && now < e.end * 1000;

  function relative(e: { start: number }): string {
    const min = Math.max(1, Math.round((e.start * 1000 - now) / 60_000));
    return min < 60 ? t('calendar.inMinutes', { n: min }) : t('calendar.inHours', { n: Math.round(min / 60) });
  }

  // Month overview
  const cells = $derived(monthGrid(data.monthFrom, todayKey));
  const monthTitle = $derived(new Intl.DateTimeFormat(locale, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(Date.UTC(+todayKey.slice(0, 4), +todayKey.slice(5, 7) - 1, 1)));
  const weekdays = $derived(
    Array.from({ length: 7 }, (_, i) => {
      const key = addDays(data.monthFrom, i);
      const [y, m, d] = key.split('-').map(Number);
      return new Intl.DateTimeFormat(locale, { weekday: 'narrow', timeZone: 'UTC' }).format(Date.UTC(y, m - 1, d));
    }),
  );
  const color = (cal: number) => data.calendars[cal]?.color ?? 'var(--accent)';
  const problems = $derived(data.calendars.filter((c) => c.error));
</script>

<div class="cal">
  <header>
    <h2>{settings.title ?? t('calendar.title')}</h2>
    {#if problems.length}
      <span class="warn" title={problems.map((c) => `${c.name}: ${c.error}`).join('\n')}>⚠ {t('calendar.outdated')}</span>
    {/if}
  </header>

  <div class="body">
    {#if showMonth}
      <section class="month" aria-label={monthTitle}>
        <div class="month-title">{monthTitle}</div>
        <div class="weekdays">{#each weekdays as w, i (i)}<span>{w}</span>{/each}</div>
        <div class="days">
          {#each cells as c (c.key)}
            <div class="day" class:out={!c.inMonth} class:today={c.today}>
              <span class="num">{c.day}</span>
              <span class="dots">
                {#each (data.marks[c.key] ?? []).slice(0, 3) as cal (cal)}<i style:background={color(cal)}></i>{/each}
              </span>
            </div>
          {/each}
        </div>
      </section>
    {/if}

    <section class="agenda">
      {#each groups.slice(0, compact ? 1 : groups.length) as g (g.key)}
        <div class="group">
          <h3 class:today={g.key === todayKey}>{dayTitle(g.key)}</h3>
          <ul>
            {#each g.events as e, i (g.key + i + e.start + e.title)}
              {@const w = when(e)}
              <li class:running={running(e)} class:next={e === next}>
                <span class="bar" style:background={color(e.cal)}></span>
                <span class="time" class:allday={e.allDay}>
                  <span class="main">{w.main}</span>
                  {#if w.sub}<span class="sub">{w.sub}</span>{/if}
                </span>
                <span class="what">
                  <span class="title">{e.title}</span>
                  {#if e.location}<span class="loc">{e.location}</span>{/if}
                </span>
                {#if running(e)}
                  <span class="tag now">{t('calendar.now')}</span>
                {:else if e === next}
                  <span class="tag">{relative(e)}</span>
                {/if}
              </li>
            {/each}
          </ul>
        </div>
      {:else}
        <div class="empty">{t('calendar.empty', { days: settings.days ?? 14 })}</div>
      {/each}
    </section>
  </div>
</div>

<style>
  .cal {
    height: 100%;
    display: flex;
    flex-direction: column;
    padding: var(--space-4) var(--space-4) var(--space-3) var(--space-5);
  }
  header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-2);
    padding-right: var(--space-2);
  }
  h2 {
    margin: 0;
    font-size: var(--text-l);
    font-weight: 600;
  }
  .warn {
    color: var(--warn);
    font-size: var(--text-s);
  }
  .body {
    flex: 1;
    min-height: 0;
    display: flex;
    gap: var(--space-5);
  }

  /* month overview */
  .month {
    flex: 0 0 min(300px, 42%);
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
  .month-title {
    font-weight: 600;
    margin-bottom: var(--space-2);
    text-transform: capitalize;
  }
  .weekdays,
  .days {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
  }
  .weekdays span {
    text-align: center;
    color: var(--muted);
    font-size: 0.72rem;
    padding-bottom: var(--space-1);
  }
  .days {
    grid-auto-rows: 46px;
    align-content: start;
  }
  .day {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 2px;
    min-height: 0;
  }
  .num {
    width: 28px;
    height: 28px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    font-size: var(--text-s);
    font-variant-numeric: tabular-nums;
  }
  .day.out .num {
    color: var(--muted);
    opacity: 0.45;
  }
  .day.today .num {
    background: var(--accent);
    color: #fff;
    font-weight: 700;
  }
  .dots {
    height: 6px;
    display: flex;
    gap: 3px;
  }
  .dots i {
    width: 5px;
    height: 5px;
    border-radius: 50%;
  }

  /* agenda */
  .agenda {
    flex: 1;
    min-width: 0;
    overflow-y: auto;
    -webkit-overflow-scrolling: touch;
    touch-action: pan-y;
    padding-right: var(--space-2);
  }
  .group + .group {
    margin-top: var(--space-3);
  }
  h3 {
    margin: 0 0 var(--space-1);
    font-size: var(--text-s);
    font-weight: 600;
    color: var(--muted);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  h3.today {
    color: var(--accent);
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-height: 52px;
    padding: var(--space-1) var(--space-2) var(--space-1) 0;
    border-radius: var(--radius-s);
  }
  li.running {
    background: var(--accent-soft);
  }
  .bar {
    flex: none;
    align-self: stretch;
    width: 4px;
    margin: var(--space-1) 0;
    border-radius: 2px;
  }
  li.next .bar {
    width: 6px;
  }
  .time {
    flex: none;
    width: 72px;
    display: flex;
    flex-direction: column;
    line-height: 1.2;
  }
  .time .main {
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .time.allday .main {
    color: var(--muted);
    font-size: 0.72rem;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .time .sub {
    color: var(--muted);
    font-size: 0.72rem;
    font-variant-numeric: tabular-nums;
  }
  .what {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .title {
    font-weight: 500;
    line-height: 1.25;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .loc {
    color: var(--muted);
    font-size: var(--text-s);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .tag {
    flex: none;
    padding: 2px var(--space-3);
    border-radius: 999px;
    background: var(--border);
    color: var(--muted);
    font-size: 0.72rem;
    font-weight: 600;
  }
  .tag.now {
    background: var(--accent);
    color: #fff;
  }
  .empty {
    height: 100%;
    display: grid;
    place-items: center;
    color: var(--muted);
    text-align: center;
  }
</style>
