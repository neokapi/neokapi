// A tiny debounce controller for the StepConfigPanel's config sync.
//
// Config edits are debounced (300ms) so we don't push a new FlowSpec on every
// keystroke. The catch (finding #51): the panel may unmount — because it's
// closed or the selection changes — while a debounced edit is still pending,
// in which case the last sub-300ms edit must be FLUSHED, not dropped. This
// controller owns the timer + the "latest pending value" so `flush()` (called
// from the React cleanup) can emit it synchronously.

export interface DebouncedSync<T> {
  /** Schedule `value` to be emitted after `delay` ms, replacing any pending one. */
  schedule: (value: T) => void;
  /** Emit any pending value immediately and cancel the timer (idempotent). */
  flush: () => void;
  /** Cancel any pending value without emitting it. */
  cancel: () => void;
  /** Whether a value is currently scheduled and not yet emitted. */
  hasPending: () => boolean;
}

export function createDebouncedSync<T>(emit: (value: T) => void, delay = 300): DebouncedSync<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let pending = false;
  let latest: T;

  const clear = () => {
    if (timer !== undefined) {
      clearTimeout(timer);
      timer = undefined;
    }
  };

  return {
    schedule(value) {
      latest = value;
      pending = true;
      clear();
      timer = setTimeout(() => {
        timer = undefined;
        pending = false;
        emit(value);
      }, delay);
    },
    flush() {
      clear();
      if (pending) {
        pending = false;
        emit(latest);
      }
    },
    cancel() {
      clear();
      pending = false;
    },
    hasPending() {
      return pending;
    },
  };
}
