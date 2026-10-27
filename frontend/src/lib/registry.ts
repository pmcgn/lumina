import type { Component } from 'svelte';
import type { WidgetStatus } from './protocol';

/** Props every widget component receives (see docs/adr/004). */
export interface WidgetProps<D = any, S = Record<string, any>> {
  id: string;
  data: D;
  settings: S;
  status: WidgetStatus;
  readOnly: boolean;
  size: { w: number; h: number; width: number; height: number }; // grid cells and pixels
  send: (action: string, params?: unknown) => Promise<{ ok: boolean; error?: string; data?: unknown }>;
}

export interface WidgetDefinition {
  /** Lazy-loaded so a widget's code is only fetched when it is used. */
  component: () => Promise<{ default: Component<any> }>;
}

const definitions = new Map<string, WidgetDefinition>();
const loaded = new Map<string, Promise<Component<any>>>();

export function registerWidget(type: string, def: WidgetDefinition) {
  definitions.set(type, def);
}

export function loadWidget(type: string): Promise<Component<any>> | undefined {
  let p = loaded.get(type);
  if (!p) {
    const def = definitions.get(type);
    if (!def) return undefined;
    p = def.component().then((m) => m.default);
    loaded.set(type, p);
  }
  return p;
}
