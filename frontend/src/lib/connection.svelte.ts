import { SvelteMap } from 'svelte/reactivity';
import type { ActionResult, ClientConfig, Envelope, WidgetState } from './protocol';

export type ConnectionStatus = 'connecting' | 'online' | 'offline';

const PING_INTERVAL = 20_000;
const SILENCE_LIMIT = 50_000; // reconnect if the server has been silent this long
const ACTION_TIMEOUT = 25_000;
const RECONNECT_MAX = 5_000;

/**
 * The single WebSocket to the backend: reconnects automatically, keeps the
 * subscriptions, and exposes config and per-widget state reactively.
 */
class Connection {
  status = $state<ConnectionStatus>('connecting');
  config = $state.raw<ClientConfig | null>(null);
  readonly widgets = new SvelteMap<string, WidgetState>();

  private ws: WebSocket | null = null;
  private attempt = 0;
  private serverVersion: string | null = null;
  private lastMessage = 0;
  private timers: number[] = [];
  private wanted = new Set<string>();
  private active = new Set<string>(); // subscribed on the current socket
  private pending = new Map<string, (r: ActionResult) => void>();
  private seq = 0;

  start() {
    this.connect();
    window.setInterval(() => this.heartbeat(), PING_INTERVAL);
  }

  /** Declares which widgets are visible; only those receive data. */
  setSubscriptions(ids: string[]) {
    this.wanted = new Set(ids);
    this.syncSubscriptions();
  }

  /** Sends a widget action; resolves with the result (never rejects). */
  send(widget: string, name: string, params?: unknown): Promise<ActionResult> {
    return new Promise((resolve) => {
      if (this.status !== 'online') return resolve({ ok: false, error: 'offline' });
      const id = `a${++this.seq}`;
      const timer = window.setTimeout(() => {
        this.pending.delete(id);
        resolve({ ok: false, error: 'timeout' });
      }, ACTION_TIMEOUT);
      this.pending.set(id, (r) => {
        window.clearTimeout(timer);
        resolve(r);
      });
      this.post({ type: 'action', widget, id, payload: { name, params } });
    });
  }

  private url() {
    return `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`;
  }

  private connect() {
    this.status = this.attempt === 0 ? 'connecting' : 'offline';
    const ws = new WebSocket(this.url());
    this.ws = ws;
    ws.onopen = () => {
      this.attempt = 0;
      this.lastMessage = Date.now();
      this.active.clear();
      this.post({ type: 'hello', payload: {} });
    };
    ws.onmessage = (ev) => {
      this.lastMessage = Date.now();
      try {
        this.handle(JSON.parse(ev.data));
      } catch (e) {
        console.error('bad message', e);
      }
    };
    ws.onclose = () => {
      if (this.ws !== ws) return;
      this.ws = null;
      this.status = 'offline';
      this.failPending();
      // Keep the cap low: after a redeploy the new backend should be picked up within seconds.
      const delay = Math.min(RECONNECT_MAX, 1000 * 2 ** this.attempt++) * (0.75 + Math.random() * 0.5);
      window.setTimeout(() => this.connect(), delay);
    };
    ws.onerror = () => ws.close();
  }

  private handle(m: Envelope) {
    switch (m.type) {
      case 'welcome': {
        const v = m.payload.serverVersion as string;
        if (this.serverVersion !== null && this.serverVersion !== v) {
          location.reload(); // new backend version deployed
          return;
        }
        this.serverVersion = v;
        this.status = 'online';
        this.syncSubscriptions();
        break;
      }
      case 'config':
        this.config = m.payload as ClientConfig;
        break;
      case 'snapshot':
      case 'update':
        if (m.widget) this.widgets.set(m.widget, m.payload as WidgetState);
        break;
      case 'action_result': {
        const done = m.id ? this.pending.get(m.id) : undefined;
        if (done && m.id) {
          this.pending.delete(m.id);
          done(m.payload as ActionResult);
        }
        break;
      }
      case 'error':
        console.warn('server error', m.widget, m.payload);
        break;
    }
  }

  private syncSubscriptions() {
    if (this.status !== 'online') return;
    const add = [...this.wanted].filter((id) => !this.active.has(id));
    const drop = [...this.active].filter((id) => !this.wanted.has(id));
    if (add.length) this.post({ type: 'subscribe', payload: { widgets: add } });
    if (drop.length) this.post({ type: 'unsubscribe', payload: { widgets: drop } });
    this.active = new Set(this.wanted);
  }

  private heartbeat() {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
    if (Date.now() - this.lastMessage > SILENCE_LIMIT) {
      this.ws.close(); // dead connection, reconnect logic takes over
      return;
    }
    this.post({ type: 'ping' });
  }

  private failPending() {
    for (const done of this.pending.values()) done({ ok: false, error: 'connection lost' });
    this.pending.clear();
  }

  private post(m: { type: string; widget?: string; id?: string; payload?: unknown }) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ v: 1, ts: Date.now(), ...m }));
    }
  }
}

export const connection = new Connection();
