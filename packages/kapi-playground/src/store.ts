// A tiny event bus that decouples the inline RunnableSnippet (SSR-clean, no
// heavy imports) from the single KapiModal mounted in Root.tsx. Calling
// `openKapi(...)` from anywhere fires the listener the modal registered; the
// modal then code-splits in its xterm/wasm payload.

/** Arguments to open the shared kapi modal. */
export interface OpenKapiOptions {
  /**
   * Command line to drop into the terminal prompt. The leading "kapi" is
   * optional. When `autoRun` is true (the default) it is executed immediately.
   */
  cmd?: string;
  /** Fixture names (see fixtures.ts) to ensure exist in the session cwd. */
  seed?: string[];
  /**
   * Optional scripted steps run in sequence after seeding. Each is a command
   * line. Forward-looking for W3's scene model; `cmd` is the common case.
   */
  steps?: string[];
  /** Run `cmd` (and `steps`) automatically on open. Defaults to true. */
  autoRun?: boolean;
}

type Listener = (opts: OpenKapiOptions) => void;

let listener: Listener | null = null;
// Buffer the most recent request if the modal host has not mounted yet (e.g.
// a Run click during hydration). Flushed when the modal registers.
let pending: OpenKapiOptions | null = null;

/** Register the single modal host. Returns an unsubscribe function. */
export function registerKapiModal(fn: Listener): () => void {
  listener = fn;
  if (pending) {
    const p = pending;
    pending = null;
    fn(p);
  }
  return () => {
    if (listener === fn) listener = null;
  };
}

/** Open the shared kapi modal. Safe to call before the modal mounts. */
export function openKapi(opts: OpenKapiOptions = {}): void {
  if (listener) listener(opts);
  else pending = opts;
}
