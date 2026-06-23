import React from "react";

// Stale-deploy chunk recovery.
//
// Docusaurus splits the site into content-hashed JS chunks. The lab explorers
// and the playground modal are loaded with `React.lazy(() => import(...))`, so
// their chunk is fetched only when the widget first renders. Our Pages deploy
// overwrites the whole `assets/` tree on every release, so a tab opened before a
// deploy references chunk filenames that no longer exist. The first time such a
// tab opens a lab modal the import 404s and React surfaces a crash.
//
// Docusaurus already force-refreshes on chunk failures during *route* navigation
// (facebook/docusaurus#7600), but that does not cover these manual `React.lazy`
// imports. This module fills that gap: detect the stale-chunk failure, retry once
// for a transient blip, then trigger a single guarded page reload. The reload
// re-fetches the new HTML + chunk manifest, so the import resolves against the
// current deploy.

const RELOAD_KEY = "kapi:stale-chunk-reload";
// Don't reload again within this window — if a reload didn't fix the import the
// chunk is genuinely unavailable, and reloading in a loop would trap the user.
const RELOAD_COOLDOWN_MS = 10_000;

/** True when an error looks like a failed dynamic-import / missing chunk. */
export function isChunkLoadError(err: unknown): boolean {
  if (err == null) return false;
  const e = err as { name?: string; message?: string; code?: string };
  if (e.name === "ChunkLoadError") return true;
  if (e.code === "CSS_CHUNK_LOAD_FAILED") return true;
  const msg = typeof e.message === "string" ? e.message : typeof err === "string" ? err : "";
  return (
    // webpack (what Docusaurus uses)
    /Loading( CSS)? chunk [\w-]+ failed/i.test(msg) ||
    /Loading chunk/i.test(msg) ||
    // Vite / native ESM imports (kapi-lab is a bundled lib)
    /Failed to fetch dynamically imported module/i.test(msg) ||
    /error loading dynamically imported module/i.test(msg) ||
    // server returned the SPA index.html for a deleted .js asset
    /is not a valid JavaScript MIME type/i.test(msg) ||
    /Unexpected token '<'/i.test(msg)
  );
}

/**
 * Reload the page once to pick up the current deploy. Guarded by sessionStorage
 * so a chunk that stays unavailable can't trap the tab in a reload loop.
 * Returns true if a reload was triggered.
 */
export function maybeReloadForStaleChunk(): boolean {
  if (typeof window === "undefined") return false;
  const now = Date.now();
  try {
    const last = Number(window.sessionStorage.getItem(RELOAD_KEY) ?? "0");
    if (Number.isFinite(last) && now - last < RELOAD_COOLDOWN_MS) return false;
    window.sessionStorage.setItem(RELOAD_KEY, String(now));
  } catch {
    // sessionStorage can be unavailable (private mode / quota). Reload anyway —
    // worst case we forgo the loop guard, which is acceptable for a one-off.
  }
  window.location.reload();
  return true;
}

async function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Run a dynamic-import factory, retrying once on a chunk-load failure (covers a
 * transient network blip) before giving up. When the chunk is genuinely gone
 * after a redeploy, trigger a guarded reload and rethrow so the surrounding
 * ChunkErrorBoundary can show a graceful "updating…" state until the reload
 * takes over.
 */
export async function retryDynamicImport<T>(
  factory: () => Promise<T>,
  retries = 1,
  delayMs = 400,
): Promise<T> {
  try {
    return await factory();
  } catch (err) {
    if (!isChunkLoadError(err)) throw err;
    if (retries > 0) {
      await sleep(delayMs);
      return retryDynamicImport(factory, retries - 1, delayMs * 2);
    }
    maybeReloadForStaleChunk();
    throw err;
  }
}

/**
 * Drop-in replacement for `React.lazy` that recovers from stale-deploy chunk
 * failures via {@link retryDynamicImport}.
 */
// Mirrors React.lazy's own signature (`T extends ComponentType<any>`) so a
// component's required props are preserved at the call site.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function lazyWithRetry<T extends React.ComponentType<any>>(
  factory: () => Promise<{ default: T }>,
): React.LazyExoticComponent<T> {
  return React.lazy(() => retryDynamicImport(factory));
}
