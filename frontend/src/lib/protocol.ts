// Types shared with the backend. The data shapes are generated from the Go
// structs (protocol.gen.ts, config.gen.ts; run `go tool tygo generate` in backend/
// after changing them). This file only gives them the names used in the frontend.

import type { Alert, WidgetState as GeneratedWidgetState } from './protocol.gen';

export type { ActionResult, Alert, AlertColor, Envelope, WidgetStatus } from './protocol.gen';
export type {
  ClientConfig,
  DisplayClient as DisplayConfig,
  Locale,
  Page as PageConfig,
  Widget as WidgetConfig,
} from './config.gen';

/** Payload of snapshot/update messages; `data` is typed by the widget that receives it. */
export interface WidgetState<T = unknown> extends Omit<GeneratedWidgetState, 'data'> {
  data?: T;
}

/** An alert as kept in the browser: `deadline` (local ms) is when it disappears by itself. */
export interface ActiveAlert extends Alert {
  deadline: number;
}
