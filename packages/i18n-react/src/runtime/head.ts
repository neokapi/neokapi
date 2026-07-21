/**
 * Document-head translatable registry (framework-free).
 *
 * The in-context review overlay anchors its inline edit affordance to visible
 * body DOM text nodes. Head strings — `<title>`, `<meta name="description">`,
 * Open Graph / Twitter card titles and descriptions — render as page metadata
 * with no text node, so the overlay can neither outline nor open them. Yet they
 * are often the highest-SEO-value strings on a page and must be reviewable.
 *
 * The `@neokapi/i18n-react/head` hooks translate a head string through the
 * runtime dictionary and register it here as they apply it to the DOM. The
 * review overlay reads this registry to list the page's head translatables in a
 * panel, each editable and saved back through the same review-store path a body
 * string uses (keyed by block hash). Nothing scrapes arbitrary head markup —
 * only strings the neokapi-i18n pipeline actually translated appear here.
 *
 * Kept plain (no React, no DOM): both the React hooks and the framework-free
 * DOM overlay import it, and the overlay must never pull React into its bundle.
 */

/**
 * Which head slot a translatable populates. The well-known kinds are the ones
 * the overlay labels; any other string is allowed (e.g. a custom `<meta>`) so
 * the registry never rejects a legitimate head string.
 */
export type HeadKind =
  | "title"
  | "description"
  | "og:title"
  | "og:description"
  | "twitter:title"
  | "twitter:description"
  // eslint-disable-next-line @typescript-eslint/ban-types
  | (string & {});

/** One head string the pipeline translated, as registered by a head hook. */
export interface HeadTranslatable {
  /** The head slot this string populates (one entry per kind per page). */
  kind: HeadKind;
  /** Block hash — the review-store key, matching the extractor's `Block.hash`. */
  hash: string;
  /** Source (default-locale) text, for the review panel's source column. */
  source: string;
}

// One entry per kind: a page has a single <title>, one description, etc.
// Re-registering a kind (e.g. on client navigation) replaces the prior entry.
const registry = new Map<HeadKind, HeadTranslatable>();
let version = 0;
const listeners = new Set<() => void>();

function notify(): void {
  version++;
  for (const fn of listeners) fn();
}

/** Register (or replace) a head translatable. Called by the head hooks. */
export function registerHeadTranslatable(entry: HeadTranslatable): void {
  const existing = registry.get(entry.kind);
  if (existing && existing.hash === entry.hash && existing.source === entry.source) return;
  registry.set(entry.kind, entry);
  notify();
}

/** Drop a head translatable — called on hook unmount so stale slots don't linger. */
export function unregisterHeadTranslatable(kind: HeadKind): void {
  if (registry.delete(kind)) notify();
}

/** Snapshot of the currently registered head translatables, in registration order. */
export function headTranslatables(): HeadTranslatable[] {
  return [...registry.values()];
}

/** Monotonic version, bumped on every registry change (for change detection). */
export function headRegistryVersion(): number {
  return version;
}

/** Subscribe to registry changes. Returns an unsubscribe function. */
export function subscribeHeadRegistry(fn: () => void): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

/** Test-only: clear the registry. */
export function __resetHeadRegistry(): void {
  registry.clear();
  notify();
}
