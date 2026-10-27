export interface Toast {
  id: number;
  message: string;
  kind: 'info' | 'error';
  actionLabel?: string;
  onAction?: () => void;
}

interface ShowOptions {
  message: string;
  kind?: 'info' | 'error';
  actionLabel?: string;
  onAction?: () => void;
  timeout?: number; // ms
}

/** Short-lived messages with an optional action such as "Undo". */
class Toasts {
  items = $state<Toast[]>([]);
  private seq = 0;

  show(o: ShowOptions) {
    const toast: Toast = { id: ++this.seq, message: o.message, kind: o.kind ?? 'info', actionLabel: o.actionLabel, onAction: o.onAction };
    this.items = [...this.items.slice(-2), toast]; // never pile up more than three
    window.setTimeout(() => this.dismiss(toast.id), o.timeout ?? (toast.kind === 'error' ? 6000 : 4000));
  }

  dismiss(id: number) {
    this.items = this.items.filter((t) => t.id !== id);
  }
}

export const toast = new Toasts();
