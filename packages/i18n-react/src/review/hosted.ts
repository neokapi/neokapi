/**
 * @neokapi/i18n-react/review/hosted — in-context review for a
 * statically-rendered site (browser).
 *
 * The dev overlay (`review/index.ts`) repaints through the app's i18n runtime
 * dictionary, so it only works on `mode: "runtime"` builds. This variant
 * repaints by patching the DOM directly and is independent of any app i18n
 * runtime — so it works on a fully-static/inline (SSG/SSR) page. It reads a
 * static `review.json` (emitted by the neokapi-i18n plugin when `review: true`,
 * or by `neokapi-i18n compile --review`), and is read-only by default:
 *
 *   - Read-only (default): a plain static deploy (GitHub/GitLab Pages) — no
 *     server needed. Deep-link, whole-page index, inspect source/target/notes.
 *   - Edit (`edit: true`): the panel becomes editable. Saving PUTs to the same
 *     review-store `{endpoint}/{hash}` protocol the dev overlay uses and patches
 *     the element's text in place — live in-context editing even on a page with
 *     no app i18n runtime. Requires a reachable review store (the dev middleware
 *     or a proxied review backend); without one, saves fail and it stays
 *     read-only. In-place repaint is text-only: blocks with inline codes or ICU
 *     plurals save to the store but are not repainted live (see `canPatchInPlace`).
 *
 * What it enables — the "click from Bowrain into the live site" flow (#38):
 *
 *   - Deep link `?kapi-focus=<hash>` on load → scrolls to the element carrying
 *     that block's `data-kapi-id`, outlines it, and opens a read-only panel
 *     (source, target for the active locale, translator note, term/check
 *     annotations). A Bowrain reviewer opens a translation unit in context in
 *     one click.
 *   - Whole-page review: `?kapi-review` on load (or clicking the toolbar pill)
 *     highlights *every* translated element at once and opens a browsable index
 *     of all strings on the page, grouped by source file — so a reviewer can
 *     scan a whole file/collection in context without first navigating to a
 *     specific block. Each row scrolls to and opens its block.
 *   - Hold ⌥/Alt and hover to outline any translated element; ⌥/Alt+click opens
 *     the same panel. A "copy context link" button yields the shareable
 *     `?kapi-focus=<hash>` URL.
 *
 * The build must stamp `data-kapi-*` attributes — pass `review: true` to the
 * neokapi-i18n plugin (it stamps in production builds too; only the dev
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
  /**
   * Query-string parameter (presence, no value needed) that opens whole-page
   * review mode on load. Default "kapi-review".
   */
  reviewParam?: string;
  /** Open the panel (not just outline) when focusing via the deep link. Default true. */
  openOnFocus?: boolean;
  /**
   * Enable in-place editing. The panel shows an editable target; saving PUTs to
   * `{endpoint}/{hash}` and patches the element's DOM text directly (no app i18n
   * runtime required). Requires a reachable review store. Default false
   * (read-only).
   */
  edit?: boolean;
  /**
   * Review-store base URL for edit saves (PUT `{endpoint}/{hash}`). Same
   * protocol the dev overlay uses. Default "/__kapi/review".
   */
  endpoint?: string;
}

let installed = false;
/** Set from options in `initKapiReviewHosted` — read by the panel. */
let editEnabled = false;
let reviewEndpoint = "/__kapi/review";

/** Idempotent bootstrap. Safe to call before the manifest exists (fetch is lazy). */
export function initKapiReviewHosted(options: HostedReviewOptions = {}): void {
  if (installed || typeof document === "undefined") return;
  installed = true;

  const focusParam = options.focusParam ?? "kapi-focus";
  const reviewParam = options.reviewParam ?? "kapi-review";
  const manifestUrl = options.manifestUrl ?? defaultManifestUrl();
  const openOnFocus = options.openOnFocus ?? true;
  editEnabled = options.edit ?? false;
  reviewEndpoint = (options.endpoint ?? "/__kapi/review").replace(/\/$/, "");

  let manifest: ReviewManifest = {};
  const getManifest = () => manifest;

  injectStyles();
  installToolbar(getManifest);
  installHoverHighlight();

  const ready = fetch(manifestUrl)
    .then((r) => (r.ok ? (r.json() as Promise<ReviewManifest>) : {}))
    .then((m) => {
      manifest = m;
    })
    .catch(() => {
      /* no manifest → outline-only, panel shows "run compile --review" */
    });

  installClickHandler(getManifest);

  const params = new URLSearchParams(location.search);

  // Deep-link focus: wait for the manifest and the target element (the SPA may
  // render it a tick after load), then scroll + outline + open.
  const focusHash = params.get(focusParam);
  if (focusHash) {
    void ready.then(() => focusBlock(focusHash, getManifest, openOnFocus));
  }

  // Whole-page review: highlight every translated element and open the index.
  if (params.has(reviewParam)) {
    void ready.then(() => enterReviewMode(getManifest));
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

// ─── Whole-page review mode ──────────────────────────────────

interface OnPageBlock {
  hash: string;
  el: Element;
  entry?: ReviewManifestEntry;
}

let reviewModeOn = false;

function toggleReviewMode(getManifest: () => ReviewManifest): void {
  if (reviewModeOn) exitReviewMode();
  else enterReviewMode(getManifest);
}

/** Highlight every stamped element on the page and open the browsable index. */
function enterReviewMode(getManifest: () => ReviewManifest): void {
  reviewModeOn = true;
  document.documentElement.setAttribute("data-kapi-review-mode", "");
  document.getElementById("kapi-review-toolbar")?.setAttribute("data-active", "");
  markUntranslated(getManifest());
  buildIndexPanel(getManifest);
}

function exitReviewMode(): void {
  reviewModeOn = false;
  document.documentElement.removeAttribute("data-kapi-review-mode");
  document.getElementById("kapi-review-toolbar")?.removeAttribute("data-active");
  document.getElementById("kapi-review-index")?.remove();
  for (const el of document.querySelectorAll("[data-kapi-review-untranslated]")) {
    el.removeAttribute("data-kapi-review-untranslated");
  }
  clearHover();
}

/** Distinct stamped elements on the page, in document order (first per hash). */
function collectOnPageBlocks(manifest: ReviewManifest): OnPageBlock[] {
  const seen = new Set<string>();
  const out: OnPageBlock[] = [];
  for (const el of document.querySelectorAll(MARKED)) {
    const hash = hashOf(el);
    if (!hash || seen.has(hash)) continue;
    seen.add(hash);
    out.push({ hash, el, entry: manifest[hash] });
  }
  return out;
}

/** Flag elements whose active-locale target is missing (falls back to source). */
function markUntranslated(manifest: ReviewManifest): void {
  const locale = activeLocale();
  for (const b of collectOnPageBlocks(manifest)) {
    if (b.entry && !b.entry.targets[locale]) {
      b.el.setAttribute("data-kapi-review-untranslated", "");
    }
  }
}

function buildIndexPanel(getManifest: () => ReviewManifest): void {
  document.getElementById("kapi-review-index")?.remove();
  const manifest = getManifest();
  const blocks = collectOnPageBlocks(manifest);
  const locale = activeLocale();

  // Group by source file, preserving document order within each group.
  const groups = new Map<string, OnPageBlock[]>();
  for (const b of blocks) {
    const file = b.entry?.properties.file ?? "(unmapped)";
    let bucket = groups.get(file);
    if (!bucket) groups.set(file, (bucket = []));
    bucket.push(b);
  }
  const translated = blocks.filter((b) => b.entry && b.entry.targets[locale]).length;

  let body = "";
  for (const [file, items] of groups) {
    body += `<div class="kapi-idx-file">${escapeHTML(file)}<span class="kapi-idx-count">${items.length}</span></div>`;
    for (const b of items) {
      const t = b.entry?.targets[locale];
      const status = !b.entry ? "unknown" : t ? "ok" : "untr";
      const text = b.entry?.source ?? b.hash;
      body +=
        `<button type="button" class="kapi-idx-row" data-hash="${escapeHTML(b.hash)}">` +
        `<span class="kapi-idx-dot kapi-${status}"></span>` +
        `<span class="kapi-idx-text">${escapeHTML(truncate(text, 90))}</span></button>`;
    }
  }

  const panel = document.createElement("div");
  panel.id = "kapi-review-index";
  panel.innerHTML = `
    <div class="kapi-idx-head">
      <div class="kapi-idx-title">kapi review <span class="kapi-idx-loc">${escapeHTML(locale || "—")}</span></div>
      <button type="button" class="kapi-idx-close" title="Exit review">✕</button>
    </div>
    <div class="kapi-idx-sub">${blocks.length} string${blocks.length === 1 ? "" : "s"} · ${translated} translated</div>
    <input class="kapi-idx-search" type="search" placeholder="Filter strings…" aria-label="Filter strings" />
    <div class="kapi-idx-list">${body || '<div class="kapi-idx-empty">No stamped strings on this page. Build with <code>review: true</code>.</div>'}</div>
  `;
  document.body.appendChild(panel);

  panel.querySelector(".kapi-idx-close")?.addEventListener("click", exitReviewMode);

  for (const row of panel.querySelectorAll<HTMLElement>(".kapi-idx-row")) {
    const hash = row.getAttribute("data-hash");
    if (!hash) continue;
    row.addEventListener("click", () => {
      for (const r of panel.querySelectorAll(".kapi-idx-row[data-active]")) {
        r.removeAttribute("data-active");
      }
      row.setAttribute("data-active", "");
      void focusBlock(hash, getManifest, true);
    });
    row.addEventListener("mouseenter", () => {
      const el = findElement(hash);
      if (el) {
        clearHover();
        el.setAttribute("data-kapi-review-hover", "");
        hovered = el;
      }
    });
    row.addEventListener("mouseleave", clearHover);
  }

  const search = panel.querySelector<HTMLInputElement>(".kapi-idx-search");
  search?.addEventListener("input", () => {
    const q = search.value.toLowerCase();
    for (const row of panel.querySelectorAll<HTMLElement>(".kapi-idx-row")) {
      row.style.display = row.textContent?.toLowerCase().includes(q) ? "" : "none";
    }
    for (const file of panel.querySelectorAll<HTMLElement>(".kapi-idx-file")) {
      // Hide a file header when all its rows are filtered out.
      let sib = file.nextElementSibling;
      let anyVisible = false;
      while (sib && sib.classList.contains("kapi-idx-row")) {
        if ((sib as HTMLElement).style.display !== "none") anyVisible = true;
        sib = sib.nextElementSibling;
      }
      file.style.display = anyVisible ? "" : "none";
    }
  });
}

function truncate(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
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

// ─── Panel ───────────────────────────────────────────────────

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
  const canEdit = editEnabled;

  const body: string[] = [];
  if (entry) {
    body.push(`<div class="kapi-label">Source</div>`);
    body.push(`<div class="kapi-source">${escapeHTML(entry.source)}</div>`);
    if (entry.properties.locNote) {
      body.push(`<div class="kapi-label">Note</div>`);
      body.push(`<div class="kapi-note">${escapeHTML(entry.properties.locNote)}</div>`);
    }
  }
  body.push(
    `<div class="kapi-label">Target <span class="kapi-locale">(${escapeHTML(locale || "—")})</span></div>`,
  );
  if (canEdit) {
    body.push(
      `<textarea id="kapi-review-edit" aria-label="Target translation">${escapeHTML(target)}</textarea>`,
    );
  } else {
    body.push(
      `<div class="kapi-target">${target ? escapeHTML(target) : '<span class="kapi-untranslated">untranslated</span>'}</div>`,
    );
  }
  if (entry) {
    body.push(renderOtherLocales(entry, locale));
    body.push(renderAnnotations(entry.annotations));
  } else if (!canEdit) {
    body.push(
      `<div class="kapi-note">No review data for this block — build with <code>review: true</code> or run <code>neokapi-i18n compile --review</code>.</div>`,
    );
  }

  const actions = canEdit
    ? `<button class="kapi-primary" id="kapi-review-save">Save</button>` +
      `<button id="kapi-review-link" title="Copy a link that opens this block in context">⧉ Link</button>` +
      `<span class="kapi-status" id="kapi-review-status"></span>`
    : `<button id="kapi-review-link" title="Copy a link that opens this block in context">⧉ Copy context link</button>` +
      `<span class="kapi-status" id="kapi-review-status">read-only</span>`;

  const panel = document.createElement("div");
  panel.id = "kapi-review-panel";
  panel.innerHTML = `
    <button class="kapi-close" title="Close">✕</button>
    <h3>${heading}</h3>
    <div class="kapi-loc">${escapeHTML(loc)} · ${escapeHTML(hash)}</div>
    ${body.join("\n")}
    <div class="kapi-row">${actions}</div>
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
  if (canEdit) {
    panel
      .querySelector("#kapi-review-save")
      ?.addEventListener("click", () => void saveEdit(hash, el, locale, manifest, panel));
  }
}

/**
 * Text-only in-place patch is safe only when the edited hash is the
 * element's own content block (not one of its `data-kapi-attr` values —
 * those are attribute strings, not text), the element has no inline
 * structure to preserve (no child elements like `<a>`/`<strong>`), and
 * the source carries no `{…}` placeholders / ICU. Anything else needs
 * structural rendering a text swap can't reproduce, so we save to the
 * store but skip the live repaint (a reload picks up the real rendering).
 */
function canPatchInPlace(el: Element, hash: string, entry?: ReviewManifestEntry): boolean {
  if (el.getAttribute("data-kapi-id") !== hash) return false;
  if (el.children.length > 0) return false;
  return !/[{}]/.test(entry?.source ?? "");
}

/**
 * PUT the edited target to the review store (`{endpoint}/{hash}`) — the same
 * protocol the dev overlay uses — then patch the DOM in place and reflect the
 * change in the in-memory manifest. Independent of any app i18n runtime.
 */
async function saveEdit(
  hash: string,
  el: Element,
  locale: string,
  manifest: ReviewManifest,
  panel: Element,
): Promise<void> {
  const status = panel.querySelector("#kapi-review-status") as HTMLElement;
  const textarea = panel.querySelector("#kapi-review-edit") as HTMLTextAreaElement | null;
  if (!textarea) return;
  const text = textarea.value;
  if (!locale) {
    status.textContent = "no active locale — switch language first";
    return;
  }
  status.textContent = "saving…";
  try {
    const res = await fetch(`${reviewEndpoint}/${hash}`, {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ locale, text }),
    });
    if (!res.ok) throw new Error(String(res.status));
  } catch (err) {
    status.textContent = `save failed (${String(err)})`;
    return;
  }
  const entry = (manifest[hash] ??= { source: "", targets: {}, properties: {}, annotations: [] });
  entry.targets[locale] = text;
  if (canPatchInPlace(el, hash, entry)) {
    el.textContent = text;
    status.textContent = "saved ✓";
  } else {
    status.textContent = "saved — inline structure not repainted live";
  }
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
      const isCheck = /qa|check|error|issue/i.test(a.type);
      return `<div class="kapi-ann${isCheck ? " kapi-check" : ""}"><strong>${escapeHTML(a.type)}</strong> ${escapeHTML(a.summary)}</div>`;
    })
    .join("");
  return `<div class="kapi-label">Annotations</div>${items}`;
}

function closePanel(): void {
  document.getElementById("kapi-review-panel")?.remove();
}

// ─── Toolbar / toast ─────────────────────────────────────────

function installToolbar(getManifest: () => ReviewManifest): void {
  const bar = document.createElement("button");
  bar.id = "kapi-review-toolbar";
  bar.type = "button";
  bar.title = "Review every string on this page in context";
  bar.innerHTML = `<span class="kapi-tb-dot"></span><span>kapi review</span><span class="kapi-ro">read-only</span>`;
  bar.addEventListener("click", () => toggleReviewMode(getManifest));
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
#kapi-review-panel textarea { width: 100%; box-sizing: border-box; min-height: 64px; border: 1px solid #d4d4d8; border-radius: 6px; padding: 8px; font: inherit; background: #fff; color: inherit; }
#kapi-review-panel button.kapi-primary { background: #4f46e5; border-color: #4f46e5; color: #fff; }
#kapi-review-panel .kapi-untranslated { color: #a1a1aa; font-style: italic; }
#kapi-review-panel .kapi-note { background: #fef9c3; border-radius: 6px; padding: 6px 8px; }
#kapi-review-panel .kapi-other { font-size: 12px; padding: 3px 0; border-bottom: 1px solid #f4f4f5; }
#kapi-review-panel .kapi-otherlang { display: inline-block; min-width: 26px; color: #71717a; font-variant: small-caps; }
#kapi-review-panel .kapi-ann { border-left: 3px solid #3b82f6; padding: 4px 8px; margin: 4px 0; background: #eff6ff; border-radius: 0 6px 6px 0; }
#kapi-review-panel .kapi-ann.kapi-check { border-left-color: #ef4444; background: #fef2f2; }
#kapi-review-panel .kapi-row { display: flex; gap: 8px; margin-top: 12px; align-items: center; }
#kapi-review-panel button { border: 1px solid #d4d4d8; background: #fafafa; border-radius: 6px; padding: 5px 12px; font: inherit; cursor: pointer; }
#kapi-review-panel .kapi-status { color: #71717a; font-size: 11px; margin-left: auto; }
#kapi-review-panel .kapi-close { position: absolute; top: 8px; right: 10px; border: none; background: none; font-size: 15px; padding: 2px 6px; }
#kapi-review-toolbar {
  position: fixed; right: 16px; bottom: 16px; z-index: 2147483645;
  background: #18181b; color: #fafafa; border: none; border-radius: 999px; padding: 6px 12px;
  font: 12px ui-sans-serif, system-ui, sans-serif; display: flex; gap: 7px; align-items: center;
  box-shadow: 0 4px 16px rgb(0 0 0 / 0.25); cursor: pointer;
}
#kapi-review-toolbar:hover { background: #27272a; }
#kapi-review-toolbar[data-active] { background: #4f46e5; }
#kapi-review-toolbar .kapi-tb-dot { width: 7px; height: 7px; border-radius: 50%; background: #a5b4fc; }
#kapi-review-toolbar[data-active] .kapi-tb-dot { background: #fff; }
#kapi-review-toolbar .kapi-ro { color: #a5b4fc; }
#kapi-review-toolbar[data-active] .kapi-ro { color: #c7d2fe; }

/* Whole-page review mode: outline every stamped element; amber = untranslated. */
html[data-kapi-review-mode] [data-kapi-id],
html[data-kapi-review-mode] [data-kapi-attr] {
  outline: 1px dashed rgb(99 102 241 / .75); outline-offset: 1px;
}
html[data-kapi-review-mode] [data-kapi-review-untranslated] {
  outline: 1.5px solid #f59e0b !important; outline-offset: 1px;
}
#kapi-review-index {
  position: fixed; top: 0; left: 0; bottom: 0; z-index: 2147483646;
  width: 320px; max-width: 82vw; display: flex; flex-direction: column;
  background: #fff; color: #111; border-right: 1px solid #d4d4d8;
  box-shadow: 4px 0 24px rgb(0 0 0 / .12);
  font: 13px/1.45 ui-sans-serif, system-ui, sans-serif;
}
#kapi-review-index .kapi-idx-head { display: flex; align-items: center; padding: 12px 14px 4px; }
#kapi-review-index .kapi-idx-title { font-weight: 600; }
#kapi-review-index .kapi-idx-loc { color: #6366f1; font-variant: small-caps; margin-left: 4px; }
#kapi-review-index .kapi-idx-close { margin-left: auto; border: none; background: none; font-size: 15px; cursor: pointer; color: inherit; padding: 2px 4px; }
#kapi-review-index .kapi-idx-sub { padding: 0 14px 8px; color: #71717a; font-size: 11px; }
#kapi-review-index .kapi-idx-search {
  margin: 0 14px 8px; padding: 6px 9px; border: 1px solid #d4d4d8; border-radius: 7px;
  font: inherit; color: inherit; background: #fafafa;
}
#kapi-review-index .kapi-idx-list { overflow: auto; padding: 0 8px 12px; }
#kapi-review-index .kapi-idx-file {
  display: flex; align-items: center; gap: 6px; font-size: 11px; font-weight: 600;
  color: #52525b; padding: 10px 6px 4px; position: sticky; top: 0; background: #fff;
}
#kapi-review-index .kapi-idx-count { margin-left: auto; color: #a1a1aa; font-weight: 400; }
#kapi-review-index .kapi-idx-row {
  display: flex; align-items: flex-start; gap: 8px; width: 100%; text-align: left;
  border: none; background: none; color: inherit; font: inherit; cursor: pointer;
  padding: 6px; border-radius: 7px;
}
#kapi-review-index .kapi-idx-row:hover { background: #f4f4f5; }
#kapi-review-index .kapi-idx-row[data-active] { background: #eef2ff; }
#kapi-review-index .kapi-idx-dot { flex: none; width: 8px; height: 8px; margin-top: 5px; border-radius: 50%; }
#kapi-review-index .kapi-idx-dot.kapi-ok { background: #22c55e; }
#kapi-review-index .kapi-idx-dot.kapi-untr { background: #f59e0b; }
#kapi-review-index .kapi-idx-dot.kapi-unknown { background: #d4d4d8; }
#kapi-review-index .kapi-idx-text { overflow: hidden; }
#kapi-review-index .kapi-idx-empty { padding: 16px 8px; color: #a1a1aa; }
#kapi-review-toast {
  position: fixed; left: 50%; bottom: 24px; transform: translateX(-50%); z-index: 2147483647;
  background: #18181b; color: #fafafa; border-radius: 8px; padding: 8px 14px;
  font: 12px ui-sans-serif, system-ui, sans-serif; box-shadow: 0 4px 16px rgb(0 0 0 / .3);
}
@media (prefers-color-scheme: dark) {
  #kapi-review-panel { background: #18181b; color: #fafafa; border-color: #3f3f46; }
  #kapi-review-panel .kapi-source { background: #27272a; }
  #kapi-review-panel .kapi-target { background: #1e1b4b; }
  #kapi-review-panel textarea { background: #27272a; border-color: #3f3f46; }
  #kapi-review-panel .kapi-note { background: #422006; }
  #kapi-review-panel .kapi-other { border-bottom-color: #27272a; }
  #kapi-review-panel .kapi-ann { background: #172554; }
  #kapi-review-panel .kapi-ann.kapi-check { background: #450a0a; }
  #kapi-review-panel button { background: #27272a; border-color: #3f3f46; }
  #kapi-review-index { background: #18181b; color: #fafafa; border-right-color: #3f3f46; }
  #kapi-review-index .kapi-idx-file { color: #a1a1aa; background: #18181b; }
  #kapi-review-index .kapi-idx-search { background: #27272a; border-color: #3f3f46; }
  #kapi-review-index .kapi-idx-row:hover { background: #27272a; }
  #kapi-review-index .kapi-idx-row[data-active] { background: #1e1b4b; }
  #kapi-review-index .kapi-idx-dot.kapi-unknown { background: #3f3f46; }
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
  // Call `escape` with `CSS` as the receiver — jsdom 30's WebIDL binding rejects
  // an unbound `escape(value)` with "'escape' called on an object that is not a
  // valid instance of CSS." Browsers accept both; the bound form is correct.
  const css = (globalThis as { CSS?: { escape?: (v: string) => string } }).CSS;
  return css?.escape ? css.escape(value) : value.replace(/["\\]/g, "\\$&");
}
