// Register every widget type here (one line per type).
import { registerWidget } from '../lib/registry';

registerWidget('core.clock', { component: () => import('./clock/Clock.svelte') });
registerWidget('weather.forecast', { component: () => import('./weather/Weather.svelte') });
registerWidget('vikunja.tasks', { component: () => import('./vikunja/Vikunja.svelte') });
registerWidget('openhab.sitemap', { component: () => import('./openhab/OpenHab.svelte') });
registerWidget('calendar.agenda', { component: () => import('./calendar/Calendar.svelte') });
registerWidget('grafana.panel', { component: () => import('./grafana/Grafana.svelte') });
