// A tiny event bus that decouples the inline RunnableSnippet (SSR-clean, no
// heavy imports) from the single KapiModal mounted in Root.tsx. Calling
// `openKapi(...)` from anywhere fires the listener the modal registered; the
// modal then code-splits in its xterm/wasm payload.

/**
 * An inline file to seed into the session cwd: dynamic content that isn't a
 * bundled fixture (e.g. a generated project.kapi, or a scene's glossary/TMX).
 * `path` may be relative (resolved against the session cwd) or absolute.
 */
export interface KapiFile {
  path: string;
  content: string;
}

/**
 * An inline binary file (e.g. a sample Office package) to seed into the session
 * cwd. Used for sample projects whose content is not UTF-8 text. Not part of the
 * shareable `?s=` snapshot (which is text-only).
 */
export interface BinaryKapiFile {
  path: string;
  bytes: Uint8Array;
}

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
   * Inline files (dynamic content not covered by a bundled fixture) to write
   * into the session cwd before the command runs. Like `seed`, existing files
   * are preserved — seeding only fills gaps.
   */
  files?: KapiFile[];
  /**
   * Inline BINARY files (e.g. a sample .docx/.xlsx) to write into the session
   * cwd before the command runs. Like `files`, existing files are preserved —
   * seeding only fills gaps.
   */
  binaryFiles?: BinaryKapiFile[];
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

// ── Shareable session state (?s= URL param) ────────────────────────────────
//
// A session is reproducible from two things: the files present in the cwd
// (captured as inline `files`, so reader edits and command outputs survive) and
// the sequence of commands that were run (replayed as `steps`). We encode that
// into a compact, URL-safe `?s=` token the modal hydrates from. Keeping this in
// the kit (not the Docusaurus host) means a plain-React host or Storybook can
// share/restore sessions too.

/** The reproducible part of a live playground session. */
export interface SessionState {
  /** Inline files (path + content) to restore into the cwd. */
  files: KapiFile[];
  /** Commands to replay, in order. */
  steps: string[];
}

const SESSION_VERSION = 1;

// base64url helpers that work in the browser without Node's Buffer. We UTF-8
// encode first so non-ASCII file content (accented pseudo-translation output)
// round-trips cleanly.
function toBase64Url(bytes: Uint8Array): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromBase64Url(s: string): Uint8Array {
  const pad = s.length % 4 === 0 ? "" : "=".repeat(4 - (s.length % 4));
  const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/") + pad);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
}

/** Encode a session into a URL-safe token suitable for `?s=`. */
export function serializeSession(state: SessionState): string {
  const payload = { v: SESSION_VERSION, f: state.files, s: state.steps };
  const json = JSON.stringify(payload);
  return toBase64Url(new TextEncoder().encode(json));
}

/**
 * Decode a `?s=` token back into a session. Returns null for malformed or
 * unknown-version tokens (forward-compatible: an older client ignores newer
 * tokens rather than crashing).
 */
export function deserializeSession(token: string): SessionState | null {
  try {
    const json = new TextDecoder().decode(fromBase64Url(token));
    const payload = JSON.parse(json) as {
      v?: number;
      f?: KapiFile[];
      s?: string[];
    };
    if (payload.v !== SESSION_VERSION) return null;
    const files = Array.isArray(payload.f)
      ? payload.f.filter(
          (x): x is KapiFile => !!x && typeof x.path === "string" && typeof x.content === "string",
        )
      : [];
    const steps = Array.isArray(payload.s) ? payload.s.filter((x) => typeof x === "string") : [];
    return { files, steps };
  } catch {
    return null;
  }
}
