/**
 * @neokapi/kapi-react/review/hosted — read-only in-context review for a
 * deployed site (browser).
 *
 * The dev overlay (`review/index.ts`) needs a Vite middleware and writes back
 * to local `.klf` files. This variant needs neither: it reads a static
 * `review.json` (emitted by `kapi-react compile --review`) and is entirely
 * read-only, so it works on a plain static deploy (GitHub/GitLab Pages).
 *
 * What it enables — the "click from Bowrain into the live site" flow (#38):
 *
 *   - Deep link `?kapi-focus=<hash>` on load → scrolls to the element carrying
 *     that block's `data-kapi-id`, outlines it, and opens a read-only panel
 *     (source, target for the active locale, translator note, term/QA
 *     annotations). A Bowrain reviewer opens a translation unit in context in
 *     one click.
 *   - Hold ⌥/Alt and hover to outline any translated element; ⌥/Alt+click opens
 *     the same panel. A "copy context link" button yields the shareable
 *     `?kapi-focus=<hash>` URL.
 *
 * The build must stamp `data-kapi-*` attributes — pass `review: true` to the
 * kapi-react plugin (it stamps in production builds too; only the dev
 * middleware/overlay injection is dev-gated).
 *
 * Framework-free plain DOM: never interferes with the host React tree.
 */

import type { ReviewManifest, ReviewManifestEntry } from "./manifest.ts";

export interface HostedReviewOptions {
  /**
   * URL of the review manifest. Default: `<base>translations/review.json`,
   * where `<base>` is derived from the document's base URI so it resolves under
   * a Pages sub-path.
   */
  manifestUrl?: string;
  /** Query-string parameter carrying the block hash to focus. Default "kapi-focus". */
  focusParam?: string;
  /** Open the panel (not just outline) when focusing via the deep link. Default true. */
  openOnFocus?: boolean;
}

let installed = false;

/** Idempotent bootstrap. Safe to call before the manifest exists (fetch is lazy). */
export function initKapiReviewHosted(options: HostedReviewOptions = {}): void {
  if (installed || typeof document === "undefined") return;
  installed = true;

  const focusParam = options.focusParam ?? "kapi-focus";
  const manifestUrl = options.manifestUrl ?? defaultManifestUrl();
  const openOnFocus = options.openOnFocus ?? true;

  injectStyles();
  installToolbar();
  installHoverHighlight();

  let manifest: ReviewManifest = {};
  const ready = fetch(manifestUrl)
    .then((r) => (r.ok ? (r.json() as Promise<ReviewManifest>) : {}))
    .then((m) => {
      manifest = m;
    })
    .catch(() => {
      /* no manifest → outline-only, panel shows "run compile --review" */
    });

  installClickHandler(() => manifest);

  // Deep-link focus: wait for the manifest and the target element (the SPA may
  // render it a tick after load), then scroll + outline + open.
  const focusHash = new URLSearchParams(location.search).get(focusParam);
  if (focusHash) {
    void ready.then(() => focusBlock(focusHash, () => manifest, openOnFocus));
  }
}

// ─── Defaults / locale ───────────────────────────────────────

function defaultManifestUrl(): string {
  try {
    return new URL("translations/review.json", document.baseURI).href;
  } catch {
    return "/translations/review.json";
  }
}

function activeLocale(entry?: ReviewManifestEntry): string {
  const lang = document.documentElement.getAttribute("lang") || "";
  if (lang && (!entry || entry.targets[lang])) return lang;
  // Fall back to the first locale that actually has a target.
  return entry ? (Object.keys(entry.targets)[0] ?? lang) : lang;
}

// ─── Element lookup ──────────────────────────────────────────

const MARKED = "[data-kapi-id], [data-kapi-attr]";

function hashOf(el: Element): string | null {
  return (
    el.getAttribute("data-kapi-id") ??
    el.getAttribute("data-kapi-attr")?.split(" ")[0]?.split(":")[1] ??
    null
  );
}

function findElement(hash: string): Element | null {
  const byId = document.querySelector(`[data-kapi-id="${cssEscape(hash)}"]`);
  if (byId) return byId;
  for (const el of document.querySelectorAll("[data-kapi-attr]")) {
    if (hashOf(el) === hash) return el;
  }
  return null;
}

/** Retry a lookup across a few frames — the target may render just after load. */
function findElementEventually(hash: string, attempts = 20): Promise<Element | null> {
  return new Promise((resolve) => {
    let n = 0;
    const tick = () => {
      const el = findElement(hash);
      if (el || n++ >= attempts) return resolve(el);
      setTimeout(tick, 100);
    };
    tick();
  });
}

// ─── Deep-link focus ─────────────────────────────────────────

async function focusBlock(
  hash: string,
  getManifest: () => ReviewManifest,
  open: boolean,
): Promise<void> {
  const el = await findElementEventually(hash);
  if (!el) {
    showToast(`kapi review: block ${hash} isn't on this page`);
    return;
  }
  el.scrollIntoView({ behavior: "smooth", block: "center" });
  el.setAttribute("data-kapi-review-focus", "");
  setTimeout(() => el.removeAttribute("data-kapi-review-focus"), 2400);
  if (open) openPanel(hash, el, getManifest());
}

// ─── Hover / click ───────────────────────────────────────────

let hovered: Element | null = null;

function installHoverHighlight(): void {
  document.addEventListener(
    "mousemove",
    (e) => {
      if (!e.altKey) return clearHover();
      const el = (e.target as Element | null)?.closest?.(MARKED) ?? null;
      if (el === hovered) return;
      clearHover();
      if (el) {
        el.setAttribute("data-kapi-review-hover", "");
        hovered = el;
      }
    },
    { capture: true, passive: true },
  );
  document.addEventListener("keyup", (e) => {
    if (e.key === "Alt") clearHover();
  });
}

function clearHover(): void {
  hovered?.removeAttribute("data-kapi-review-hover");
  hovered = null;
}

function installClickHandler(getManifest: () => ReviewManifest): void {
  document.addEventListener(
    "click",
    (e) => {
      if (!e.altKey) return;
      const el = (e.target as Element | null)?.closest?.(MARKED);
      if (!el) return;
      const hash = hashOf(el);
      if (!hash) return;
      e.preventDefault();
      e.stopPropagation();
      openPanel(hash, el, getManifest());
    },
    { capture: true },
  );
}

// ─── Panel (read-only) ───────────────────────────────────────

function openPanel(hash: string, el: Element, manifest: ReviewManifest): void {
  closePanel();
  const entry = manifest[hash];
  const locale = activeLocale(entry);
  const loc =
    el.getAttribute("data-kapi-loc") ??
    (entry?.properties.file ? `${entry.properties.file}:${entry.properties.line ?? ""}` : hash);
  const target = entry?.targets[locale] ?? "";
  const heading = entry?.properties.element
    ? `&lt;${escapeHTML(entry.properties.element)}&gt;${
        entry.properties.component ? ` · ${escapeHTML(entry.properties.component)}` : ""
      }`
    : "kapi review";

  const panel = document.createElement("div");
  panel.id = "kapi-review-panel";
  panel.innerHTML = `
    <button class="kapi-close" title="Close">✕</button>
    <h3>${heading}</h3>
    <div class="kapi-loc">${escapeHTML(loc)} · ${escapeHTML(hash)}</div>
    ${
      entry
        ? `
      <div class="kapi-label">Source</div>
      <div class="kapi-source">${escapeHTML(entry.source)}</div>
      ${entry.properties.locNote ? `<div class="kapi-label">Note</div><div class="kapi-note">${escapeHTML(entry.properties.locNote)}</div>` : ""}
      <div class="kapi-label">Target <span class="kapi-locale">(${escapeHTML(locale || "—")})</span></div>
      <div class="kapi-target">${target ? escapeHTML(target) : '<span class="kapi-untranslated">untranslated</span>'}</div>
      ${renderOtherLocales(entry, locale)}
      ${renderAnnotations(entry.annotations)}`
        : `<div class="kapi-note">No review data for this block — run <code>kapi-react compile --review</code>.</div>`
    }
    <div class="kapi-row">
      <button id="kapi-review-link" title="Copy a link that opens this block in context">⧉ Copy context link</button>
      <span class="kapi-status" id="kapi-review-status">read-only</span>
    </div>
  `;
  document.body.appendChild(panel);

  panel.querySelector(".kapi-close")?.addEventListener("click", closePanel);
  panel.querySelector("#kapi-review-link")?.addEventListener("click", () => {
    const url = new URL(location.href);
    url.searchParams.set("kapi-focus", hash);
    const status = panel.querySelector("#kapi-review-status") as HTMLElement;
    void copyText(url.href).then(
      () => (status.textContent = "link copied ✓"),
      () => (status.textContent = url.href),
    );
  });
}

function renderOtherLocales(entry: ReviewManifestEntry, active: string): string {
  const others = Object.entries(entry.targets).filter(([l]) => l !== active);
  if (others.length === 0) return "";
  const rows = others
    .map(
      ([l, t]) =>
        `<div class="kapi-other"><span class="kapi-otherlang">${escapeHTML(l)}</span> ${escapeHTML(t)}</div>`,
    )
    .join("");
  return `<div class="kapi-label">Other locales</div>${rows}`;
}

function renderAnnotations(annotations: ReviewManifestEntry["annotations"]): string {
  if (!annotations || annotations.length === 0) return "";
  const items = annotations
    .map((a) => {
      const qa = /qa|check|error|issue/i.test(a.type);
      return `<div class="kapi-ann${qa ? " kapi-qa" : ""}"><strong>${escapeHTML(a.type)}</strong> ${escapeHTML(a.summary)}</div>`;
    })
    .join("");
  return `<div class="kapi-label">Annotations</div>${items}`;
}

function closePanel(): void {
  document.getElementById("kapi-review-panel")?.remove();
}

// ─── Toolbar / toast ─────────────────────────────────────────

function installToolbar(): void {
  const bar = document.createElement("div");
  bar.id = "kapi-review-toolbar";
  bar.innerHTML = `<span>⌥ kapi review</span><span class="kapi-ro">read-only</span>`;
  document.body.appendChild(bar);
}

let toastTimer: ReturnType<typeof setTimeout> | undefined;

function showToast(message: string): void {
  let toast = document.getElementById("kapi-review-toast");
  if (!toast) {
    toast = document.createElement("div");
    toast.id = "kapi-review-toast";
    document.body.appendChild(toast);
  }
  toast.textContent = message;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast?.remove(), 3200);
}

function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) return navigator.clipboard.writeText(text);
  return Promise.reject(new Error("clipboard unavailable"));
}

// ─── Styles ──────────────────────────────────────────────────

const STYLES = `
[data-kapi-review-hover] { outline: 2px solid #6366f1 !important; outline-offset: 1px; cursor: context-menu !important; }
[data-kapi-review-focus] { outline: 2px solid #4f46e5 !important; outline-offset: 2px; border-radius: 3px; animation: kapi-pulse 1.2s ease-out 2; }
@keyframes kapi-pulse { 0% { box-shadow: 0 0 0 0 rgb(79 70 229 / .5); } 100% { box-shadow: 0 0 0 10px rgb(79 70 229 / 0); } }
#kapi-review-panel {
  position: fixed; right: 16px; bottom: 16px; z-index: 2147483646;
  width: 380px; max-height: 72vh; overflow: auto;
  background: #fff; color: #111; border: 1px solid #d4d4d8; border-radius: 10px;
  box-shadow: 0 8px 30px rgb(0 0 0 / 0.18);
  font: 13px/1.45 ui-sans-serif, system-ui, sans-serif; padding: 14px;
}
#kapi-review-panel h3 { margin: 0 0 2px; font-size: 13px; font-weight: 600; }
#kapi-review-panel .kapi-loc { color: #71717a; font-size: 11px; margin-bottom: 10px; word-break: break-all; }
#kapi-review-panel .kapi-label { font-size: 11px; text-transform: uppercase; letter-spacing: .04em; color: #71717a; margin: 10px 0 3px; }
#kapi-review-panel .kapi-source, #kapi-review-panel .kapi-target { white-space: pre-wrap; background: #f4f4f5; border-radius: 6px; padding: 8px; }
#kapi-review-panel .kapi-target { background: #eef2ff; }
#kapi-review-panel .kapi-untranslated { color: #a1a1aa; font-style: italic; }
#kapi-review-panel .kapi-note { background: #fef9c3; border-radius: 6px; padding: 6px 8px; }
#kapi-review-panel .kapi-other { font-size: 12px; padding: 3px 0; border-bottom: 1px solid #f4f4f5; }
#kapi-review-panel .kapi-otherlang { display: inline-block; min-width: 26px; color: #71717a; font-variant: small-caps; }
#kapi-review-panel .kapi-ann { border-left: 3px solid #3b82f6; padding: 4px 8px; margin: 4px 0; background: #eff6ff; border-radius: 0 6px 6px 0; }
#kapi-review-panel .kapi-ann.kapi-qa { border-left-color: #ef4444; background: #fef2f2; }
#kapi-review-panel .kapi-row { display: flex; gap: 8px; margin-top: 12px; align-items: center; }
#kapi-review-panel button { border: 1px solid #d4d4d8; background: #fafafa; border-radius: 6px; padding: 5px 12px; font: inherit; cursor: pointer; }
#kapi-review-panel .kapi-status { color: #71717a; font-size: 11px; margin-left: auto; }
#kapi-review-panel .kapi-close { position: absolute; top: 8px; right: 10px; border: none; background: none; font-size: 15px; padding: 2px 6px; }
#kapi-review-toolbar {
  position: fixed; right: 16px; bottom: 16px; z-index: 2147483645;
  background: #18181b; color: #fafafa; border-radius: 999px; padding: 6px 12px;
  font: 12px ui-sans-serif, system-ui, sans-serif; display: flex; gap: 8px; align-items: center;
  box-shadow: 0 4px 16px rgb(0 0 0 / 0.25);
}
#kapi-review-toolbar .kapi-ro { color: #a5b4fc; }
#kapi-review-toast {
  position: fixed; left: 50%; bottom: 24px; transform: translateX(-50%); z-index: 2147483647;
  background: #18181b; color: #fafafa; border-radius: 8px; padding: 8px 14px;
  font: 12px ui-sans-serif, system-ui, sans-serif; box-shadow: 0 4px 16px rgb(0 0 0 / .3);
}
@media (prefers-color-scheme: dark) {
  #kapi-review-panel { background: #18181b; color: #fafafa; border-color: #3f3f46; }
  #kapi-review-panel .kapi-source { background: #27272a; }
  #kapi-review-panel .kapi-target { background: #1e1b4b; }
  #kapi-review-panel .kapi-note { background: #422006; }
  #kapi-review-panel .kapi-other { border-bottom-color: #27272a; }
  #kapi-review-panel .kapi-ann { background: #172554; }
  #kapi-review-panel .kapi-ann.kapi-qa { background: #450a0a; }
  #kapi-review-panel button { background: #27272a; border-color: #3f3f46; }
}
`;

function injectStyles(): void {
  if (document.getElementById("kapi-review-hosted-styles")) return;
  const style = document.createElement("style");
  style.id = "kapi-review-hosted-styles";
  style.textContent = STYLES;
  document.head.appendChild(style);
}

// ─── Small helpers ───────────────────────────────────────────

function escapeHTML(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function cssEscape(value: string): string {
  const fn = (globalThis as { CSS?: { escape?: (v: string) => string } }).CSS?.escape;
  return fn ? fn(value) : value.replace(/["\\]/g, "\\$&");
}
