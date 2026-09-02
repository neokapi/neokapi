/**
 * @neokapi/i18n-react/review — in-context review overlay (browser).
 *
 * Works against the review middleware the Vite plugin mounts in dev
 * (`review: true`). The build-time transform stamps every extracted
 * element with `data-kapi-id` / `data-kapi-attr` / `data-kapi-loc`;
 * this module turns those stamps into an interactive layer:
 *
 *   - Hold ⌥/Alt: hovering a translated element outlines it.
 *   - ⌥/Alt+click: opens the review panel — source text, editable
 *     target for the active locale, translator note, term/check
 *     annotations. Saving writes back into the local `.kbf.json` file
 *     (git-diffable review) and repaints the live UI in place.
 *   - "Highlight terms & checks": paints run-level term matches and
 *     findings onto the live page via the CSS Custom Highlight API —
 *     zero DOM mutation, no layout shift.
 *
 * Deliberately framework-free (plain DOM): the overlay must never
 * interfere with the host app's React tree.
 */

import { __t, refreshTranslations, setTranslations } from "../runtime/index.ts";
import {
  headTranslatables,
  subscribeHeadRegistry,
  type HeadTranslatable,
} from "../runtime/head.ts";

export interface ReviewOptions {
  /** Review middleware base path. Default: "/__kapi/review". */
  endpoint?: string;
}

interface ReviewPayload {
  hash: string;
  sourceText: string;
  targets: Record<string, { text: string }>;
  properties: { file: string; line: number; component: string; element: string; locNote?: string };
  annotations: Array<{ id: string; annotationType: string; data: unknown }>;
}

let installed = false;

/** Idempotent overlay bootstrap. */
export function initKapiReview(options: ReviewOptions = {}): void {
  if (installed || typeof document === "undefined") return;
  installed = true;
  const endpoint = (options.endpoint ?? "/__kapi/review").replace(/\/$/, "");

  injectStyles();
  installHoverHighlight();
  installClickHandler(endpoint);
  subscribeToUpdates(endpoint);
  installToolbar(endpoint);
  installHeadPanel(endpoint);
}

// ─── Locale ──────────────────────────────────────────────────

/**
 * The active target locale: `<html lang>` (kept in sync by the
 * runtime's setTranslations) unless it's the source locale shape we
 * can't review against, in which case the panel lets you type one.
 */
function activeLocale(): string {
  return document.documentElement.getAttribute("lang") || "";
}

// ─── Styling ─────────────────────────────────────────────────

const STYLES = `
[data-kapi-review-hover] { outline: 2px solid #6366f1 !important; outline-offset: 1px; cursor: context-menu !important; }
#kapi-review-panel {
  position: fixed; right: 16px; bottom: 16px; z-index: 2147483646;
  width: 380px; max-height: 70vh; overflow: auto;
  background: #fff; color: #111; border: 1px solid #d4d4d8; border-radius: 10px;
  box-shadow: 0 8px 30px rgb(0 0 0 / 0.18);
  font: 13px/1.45 ui-sans-serif, system-ui, sans-serif; padding: 14px;
}
#kapi-review-panel h3 { margin: 0 0 2px; font-size: 13px; font-weight: 600; }
#kapi-review-panel .kapi-loc { color: #71717a; font-size: 11px; margin-bottom: 10px; word-break: break-all; }
#kapi-review-panel .kapi-label { font-size: 11px; text-transform: uppercase; letter-spacing: .04em; color: #71717a; margin: 10px 0 3px; }
#kapi-review-panel .kapi-source { white-space: pre-wrap; background: #f4f4f5; border-radius: 6px; padding: 8px; }
#kapi-review-panel textarea { width: 100%; box-sizing: border-box; min-height: 64px; border: 1px solid #d4d4d8; border-radius: 6px; padding: 8px; font: inherit; background: #fff; color: inherit; }
#kapi-review-panel .kapi-note { background: #fef9c3; border-radius: 6px; padding: 6px 8px; }
#kapi-review-panel .kapi-ann { border-left: 3px solid #3b82f6; padding: 4px 8px; margin: 4px 0; background: #eff6ff; border-radius: 0 6px 6px 0; }
#kapi-review-panel .kapi-ann.kapi-qa { border-left-color: #ef4444; background: #fef2f2; }
#kapi-review-panel .kapi-row { display: flex; gap: 8px; margin-top: 12px; align-items: center; }
#kapi-review-panel button { border: 1px solid #d4d4d8; background: #fafafa; border-radius: 6px; padding: 5px 12px; font: inherit; cursor: pointer; }
#kapi-review-panel button.kapi-primary { background: #4f46e5; border-color: #4f46e5; color: #fff; }
#kapi-review-panel .kapi-status { color: #71717a; font-size: 11px; margin-left: auto; }
#kapi-review-panel .kapi-close { position: absolute; top: 8px; right: 10px; border: none; background: none; font-size: 15px; padding: 2px 6px; }
#kapi-review-toolbar {
  position: fixed; right: 16px; bottom: 16px; z-index: 2147483645;
  background: #18181b; color: #fafafa; border-radius: 999px; padding: 6px 12px;
  font: 12px ui-sans-serif, system-ui, sans-serif; display: flex; gap: 10px; align-items: center;
  box-shadow: 0 4px 16px rgb(0 0 0 / 0.25); cursor: default;
}
#kapi-review-toolbar button { border: none; background: none; color: inherit; font: inherit; cursor: pointer; opacity: .85; }
#kapi-review-toolbar button:hover { opacity: 1; }
#kapi-review-toolbar .kapi-on { color: #a5b4fc; }
#kapi-review-head-panel {
  position: fixed; left: 16px; bottom: 16px; z-index: 2147483646;
  width: 380px; max-height: 72vh; overflow: auto;
  background: #fff; color: #111; border: 1px solid #d4d4d8; border-radius: 10px;
  box-shadow: 0 8px 30px rgb(0 0 0 / 0.18);
  font: 13px/1.45 ui-sans-serif, system-ui, sans-serif; padding: 14px;
}
#kapi-review-head-panel h3 { margin: 0 0 2px; font-size: 13px; font-weight: 600; }
#kapi-review-head-panel .kapi-loc { color: #71717a; font-size: 11px; margin-bottom: 10px; }
#kapi-review-head-panel .kapi-label { font-size: 11px; text-transform: uppercase; letter-spacing: .04em; color: #71717a; margin: 8px 0 3px; }
#kapi-review-head-panel .kapi-source { white-space: pre-wrap; background: #f4f4f5; border-radius: 6px; padding: 8px; }
#kapi-review-head-panel textarea { width: 100%; box-sizing: border-box; min-height: 48px; border: 1px solid #d4d4d8; border-radius: 6px; padding: 8px; font: inherit; background: #fff; color: inherit; }
#kapi-review-head-panel .kapi-row { display: flex; gap: 8px; margin-top: 8px; align-items: center; }
#kapi-review-head-panel button { border: 1px solid #d4d4d8; background: #fafafa; border-radius: 6px; padding: 5px 12px; font: inherit; cursor: pointer; }
#kapi-review-head-panel button.kapi-primary { background: #4f46e5; border-color: #4f46e5; color: #fff; }
#kapi-review-head-panel .kapi-status { color: #71717a; font-size: 11px; margin-left: auto; }
#kapi-review-head-panel .kapi-close { position: absolute; top: 8px; right: 10px; border: none; background: none; font-size: 15px; padding: 2px 6px; }
#kapi-review-head-panel .kapi-head-entry { padding: 8px 0; border-top: 1px solid #f4f4f5; }
#kapi-review-head-panel .kapi-head-entry:first-child { border-top: none; }
#kapi-review-head-panel .kapi-head-kind { font-size: 11px; font-weight: 600; color: #4f46e5; font-variant: small-caps; letter-spacing: .02em; }
@media (prefers-color-scheme: dark) {
  #kapi-review-panel { background: #18181b; color: #fafafa; border-color: #3f3f46; }
  #kapi-review-panel .kapi-source { background: #27272a; }
  #kapi-review-panel textarea { background: #27272a; border-color: #3f3f46; }
  #kapi-review-panel .kapi-note { background: #422006; }
  #kapi-review-panel .kapi-ann { background: #172554; }
  #kapi-review-panel .kapi-ann.kapi-qa { background: #450a0a; }
  #kapi-review-panel button { background: #27272a; border-color: #3f3f46; }
  #kapi-review-head-panel { background: #18181b; color: #fafafa; border-color: #3f3f46; }
  #kapi-review-head-panel .kapi-source { background: #27272a; }
  #kapi-review-head-panel textarea { background: #27272a; border-color: #3f3f46; }
  #kapi-review-head-panel button { background: #27272a; border-color: #3f3f46; }
  #kapi-review-head-panel button.kapi-primary { background: #4f46e5; border-color: #4f46e5; }
  #kapi-review-head-panel .kapi-head-kind { color: #a5b4fc; }
  #kapi-review-head-panel .kapi-head-entry { border-top-color: #27272a; }
}
::highlight(kapi-term) { background-color: rgb(59 130 246 / 0.28); text-decoration: underline; text-decoration-color: #3b82f6; }
::highlight(kapi-qa) { background-color: rgb(239 68 68 / 0.22); text-decoration: underline; text-decoration-style: wavy; text-decoration-color: #ef4444; }
`;

function injectStyles(): void {
  const style = document.createElement("style");
  style.id = "kapi-review-styles";
  style.textContent = STYLES;
  document.head.appendChild(style);
}

// ─── Hover + click ───────────────────────────────────────────

const MARKED = "[data-kapi-id], [data-kapi-attr]";
let hovered: Element | null = null;

function installHoverHighlight(): void {
  document.addEventListener(
    "mousemove",
    (e) => {
      if (!e.altKey) {
        clearHover();
        return;
      }
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

function installClickHandler(endpoint: string): void {
  document.addEventListener(
    "click",
    (e) => {
      if (!e.altKey) return;
      const el = (e.target as Element | null)?.closest?.(MARKED);
      if (!el) return;
      e.preventDefault();
      e.stopPropagation();
      const hash =
        el.getAttribute("data-kapi-id") ??
        el.getAttribute("data-kapi-attr")?.split(" ")[0]?.split(":")[1];
      if (hash) void openPanel(endpoint, hash, el);
    },
    { capture: true },
  );
}

// ─── Panel ───────────────────────────────────────────────────

async function openPanel(endpoint: string, hash: string, el: Element): Promise<void> {
  let payload: ReviewPayload;
  try {
    const res = await fetch(`${endpoint}/${hash}`);
    if (!res.ok) throw new Error(`${res.status}`);
    payload = (await res.json()) as ReviewPayload;
  } catch {
    showToast("kapi review: block not found in the KBF tree — run extract first");
    return;
  }

  closePanel();
  const locale = activeLocale();
  const target = payload.targets[locale]?.text ?? "";
  const loc =
    el.getAttribute("data-kapi-loc") ?? `${payload.properties.file}:${payload.properties.line}`;

  const panel = document.createElement("div");
  panel.id = "kapi-review-panel";
  panel.innerHTML = `
    <button class="kapi-close" title="Close">✕</button>
    <h3>&lt;${escapeHTML(payload.properties.element)}&gt; · ${escapeHTML(payload.properties.component)}</h3>
    <div class="kapi-loc">${escapeHTML(loc)} · ${escapeHTML(hash)}</div>
    <div class="kapi-label">Source</div>
    <div class="kapi-source">${escapeHTML(payload.sourceText)}</div>
    ${payload.properties.locNote ? `<div class="kapi-label">Note</div><div class="kapi-note">${escapeHTML(payload.properties.locNote)}</div>` : ""}
    <div class="kapi-label">Target <span class="kapi-locale">(${escapeHTML(locale || "set a locale")})</span></div>
    <textarea id="kapi-review-target">${escapeHTML(target)}</textarea>
    ${renderAnnotations(payload.annotations)}
    <div class="kapi-row">
      <button class="kapi-primary" id="kapi-review-save">Save to .kbf.json</button>
      <span class="kapi-status" id="kapi-review-status"></span>
    </div>
  `;
  document.body.appendChild(panel);

  panel.querySelector(".kapi-close")?.addEventListener("click", closePanel);
  panel.querySelector("#kapi-review-save")?.addEventListener("click", () => {
    const text = (panel.querySelector("#kapi-review-target") as HTMLTextAreaElement).value;
    const status = panel.querySelector("#kapi-review-status") as HTMLElement;
    void saveTargetEdit(endpoint, hash, locale, text, status);
  });
}

/**
 * Write one target edit back through the review store — the single path every
 * edit affordance in the overlay (inline block panel and head panel alike)
 * takes. PUTs `{ locale, text }` to `{endpoint}/{hash}`, then optimistically
 * merges the edit into the runtime dict and repaints in place without waiting
 * for the SSE echo. `status`, when given, narrates progress.
 */
async function saveTargetEdit(
  endpoint: string,
  hash: string,
  locale: string,
  text: string,
  status?: HTMLElement | null,
): Promise<boolean> {
  if (!locale) {
    if (status) status.textContent = "no active locale — switch language first";
    return false;
  }
  if (status) status.textContent = "saving…";
  try {
    const res = await fetch(`${endpoint}/${hash}`, {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ locale, text }),
    });
    if (!res.ok) throw new Error(`${res.status}`);
    setTranslations(locale, { [hash]: text }, { merge: true });
    refreshTranslations();
    if (status) status.textContent = "saved ✓";
    return true;
  } catch (err) {
    if (status) status.textContent = `save failed (${String(err)})`;
    return false;
  }
}

function renderAnnotations(annotations: ReviewPayload["annotations"]): string {
  if (annotations.length === 0) return "";
  const items = annotations
    .map((a) => {
      const qa = /qa|check|error|issue/i.test(a.annotationType);
      const summary = annotationSummary(a.data) ?? a.annotationType;
      return `<div class="kapi-ann${qa ? " kapi-qa" : ""}"><strong>${escapeHTML(shortType(a.annotationType))}</strong> ${escapeHTML(summary)}</div>`;
    })
    .join("");
  return `<div class="kapi-label">Annotations</div>${items}`;
}

function shortType(t: string): string {
  return t.split("/").pop() ?? t;
}

function annotationSummary(data: unknown): string | null {
  if (typeof data === "string") return data;
  if (typeof data === "number" || typeof data === "boolean") return JSON.stringify(data);
  if (data === null || typeof data !== "object") return null;
  const d = data as Record<string, unknown>;
  for (const key of ["message", "term", "text", "match", "note", "summary"]) {
    if (typeof d[key] === "string") return d[key] as string;
  }
  return null;
}

function closePanel(): void {
  document.getElementById("kapi-review-panel")?.remove();
}

// ─── Term / check highlight painting ─────────────────────────

let highlightsOn = false;

async function toggleHighlights(endpoint: string, button: HTMLElement): Promise<void> {
  const highlights = (CSS as unknown as { highlights?: Map<string, unknown> }).highlights;
  if (!highlights) {
    showToast("kapi review: CSS Custom Highlight API unavailable in this browser");
    return;
  }
  if (highlightsOn) {
    highlights.delete("kapi-term");
    highlights.delete("kapi-qa");
    highlightsOn = false;
    button.classList.remove("kapi-on");
    return;
  }
  let byHash: Record<string, Array<{ annotationType: string; data: unknown }>>;
  try {
    const res = await fetch(`${endpoint}/annotations`);
    byHash = (await res.json()) as typeof byHash;
  } catch {
    showToast("kapi review: middleware unreachable");
    return;
  }

  const HighlightCtor = (globalThis as Record<string, unknown>).Highlight as
    | (new (...ranges: Range[]) => unknown)
    | undefined;
  if (!HighlightCtor) return;
  const termRanges: Range[] = [];
  const qaRanges: Range[] = [];

  for (const el of document.querySelectorAll(MARKED)) {
    const hash =
      el.getAttribute("data-kapi-id") ??
      el.getAttribute("data-kapi-attr")?.split(" ")[0]?.split(":")[1];
    const annotations = hash ? byHash[hash] : undefined;
    if (!annotations) continue;
    for (const a of annotations) {
      const needle = annotationSummaryForMatch(a.data);
      if (!needle) continue;
      const qa = /qa|check|error|issue/i.test(a.annotationType);
      for (const range of findTextRanges(el, needle)) {
        (qa ? qaRanges : termRanges).push(range);
      }
    }
  }

  if (termRanges.length > 0) highlights.set("kapi-term", new HighlightCtor(...termRanges));
  if (qaRanges.length > 0) highlights.set("kapi-qa", new HighlightCtor(...qaRanges));
  highlightsOn = true;
  button.classList.add("kapi-on");
  showToast(`kapi review: ${termRanges.length} term / ${qaRanges.length} check highlights`);
}

/** The string to locate in the rendered text for an annotation. */
function annotationSummaryForMatch(data: unknown): string | null {
  if (data === null || typeof data !== "object") return null;
  const d = data as Record<string, unknown>;
  for (const key of ["term", "match", "text", "target"]) {
    if (typeof d[key] === "string" && (d[key] as string).length > 1) return d[key] as string;
  }
  return null;
}

/** All occurrences of `needle` across the element's text nodes. */
function findTextRanges(el: Element, needle: string): Range[] {
  const ranges: Range[] = [];
  const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
  const lower = needle.toLowerCase();
  let node: Node | null;
  while ((node = walker.nextNode())) {
    const text = node.textContent ?? "";
    const hay = text.toLowerCase();
    let from = 0;
    let at: number;
    while ((at = hay.indexOf(lower, from)) >= 0) {
      const range = document.createRange();
      range.setStart(node, at);
      range.setEnd(node, at + needle.length);
      ranges.push(range);
      from = at + needle.length;
    }
  }
  return ranges;
}

// ─── Live updates (SSE) ──────────────────────────────────────

function subscribeToUpdates(endpoint: string): void {
  if (typeof EventSource === "undefined") return;
  const source = new EventSource(`${endpoint}/events`);
  source.onmessage = (event) => {
    try {
      const { hash, locale, text } = JSON.parse(event.data as string) as {
        hash: string;
        locale: string;
        text: string;
      };
      if (locale === activeLocale()) {
        setTranslations(locale, { [hash]: text }, { merge: true });
        refreshTranslations();
      }
    } catch {
      // Ignore malformed events.
    }
  };
}

// ─── Toolbar + toast ─────────────────────────────────────────

function installToolbar(endpoint: string): void {
  const bar = document.createElement("div");
  bar.id = "kapi-review-toolbar";
  bar.innerHTML =
    `<span>⌥ kapi review</span>` +
    `<button id="kapi-review-hl" title="Highlight terms & checks">terms/checks</button>` +
    `<button id="kapi-review-head-btn" title="Review head & SEO strings (title, meta, og)" hidden>head/SEO</button>`;
  document.body.appendChild(bar);
  const button = bar.querySelector("#kapi-review-hl") as HTMLElement;
  button.addEventListener("click", () => void toggleHighlights(endpoint, button));
}

// ─── Head / SEO strings panel ────────────────────────────────

/**
 * List the page's document-head translatables — `<title>`, `<meta>` description,
 * Open Graph / Twitter cards set through the `@neokapi/i18n-react/head` hooks —
 * in a panel, each with the same source/target/edit/save control the inline
 * affordance uses, wired through the same review store. Head strings have no DOM
 * text node to anchor inline, so this panel is how a reviewer reaches them.
 *
 * A no-op when there are no head translatables: the toolbar button stays hidden
 * and no panel is shown until a head hook registers one.
 */
function installHeadPanel(endpoint: string): void {
  const button = document.getElementById("kapi-review-head-btn") as HTMLButtonElement | null;
  if (!button) return;

  const syncButton = (): void => {
    const count = headTranslatables().length;
    button.hidden = count === 0;
    button.textContent = count > 0 ? `head/SEO (${count})` : "head/SEO";
    // A registry change (e.g. client navigation) refreshes an open panel.
    if (document.getElementById("kapi-review-head-panel")) renderHeadPanel(endpoint);
  };

  button.addEventListener("click", () => {
    if (document.getElementById("kapi-review-head-panel")) closeHeadPanel();
    else renderHeadPanel(endpoint);
  });

  subscribeHeadRegistry(syncButton);
  syncButton();
}

function renderHeadPanel(endpoint: string): void {
  closeHeadPanel();
  const entries = headTranslatables();
  if (entries.length === 0) return;
  const locale = activeLocale();

  const panel = document.createElement("div");
  panel.id = "kapi-review-head-panel";
  panel.innerHTML = `
    <button class="kapi-close" title="Close">✕</button>
    <h3>Head &amp; SEO strings</h3>
    <div class="kapi-loc">Not rendered in the page body — title, description, og/twitter · ${escapeHTML(
      locale || "set a locale",
    )}</div>
    <div class="kapi-head-list">${entries.map((e) => renderHeadEntry(e, locale)).join("")}</div>
  `;
  document.body.appendChild(panel);

  panel.querySelector(".kapi-close")?.addEventListener("click", closeHeadPanel);
  for (const row of panel.querySelectorAll<HTMLElement>(".kapi-head-entry")) {
    const hash = row.getAttribute("data-hash");
    if (!hash) continue;
    row.querySelector(".kapi-head-save")?.addEventListener("click", () => {
      const text = (row.querySelector(".kapi-head-target") as HTMLTextAreaElement).value;
      const status = row.querySelector(".kapi-head-status") as HTMLElement;
      void saveTargetEdit(endpoint, hash, locale, text, status);
    });
  }
}

function renderHeadEntry(entry: HeadTranslatable, locale: string): string {
  // The active-locale value is whatever the runtime dict resolves — the same
  // string the head hook applied to the live page (source when untranslated).
  const target = __t(entry.hash, entry.source);
  return `
    <div class="kapi-head-entry" data-hash="${escapeHTML(entry.hash)}">
      <div class="kapi-head-kind">${escapeHTML(String(entry.kind))}</div>
      <div class="kapi-label">Source</div>
      <div class="kapi-source">${escapeHTML(entry.source)}</div>
      <div class="kapi-label">Target <span class="kapi-locale">(${escapeHTML(locale || "set a locale")})</span></div>
      <textarea class="kapi-head-target">${escapeHTML(target)}</textarea>
      <div class="kapi-row">
        <button class="kapi-primary kapi-head-save">Save to .kbf.json</button>
        <span class="kapi-status kapi-head-status"></span>
      </div>
    </div>`;
}

function closeHeadPanel(): void {
  document.getElementById("kapi-review-head-panel")?.remove();
}

let toastTimer: ReturnType<typeof setTimeout> | undefined;

function showToast(message: string): void {
  let toast = document.getElementById("kapi-review-toast");
  if (!toast) {
    toast = document.createElement("div");
    toast.id = "kapi-review-toast";
    toast.style.cssText =
      "position:fixed;left:50%;bottom:24px;transform:translateX(-50%);z-index:2147483647;" +
      "background:#18181b;color:#fafafa;border-radius:8px;padding:8px 14px;" +
      "font:12px ui-sans-serif,system-ui,sans-serif;box-shadow:0 4px 16px rgb(0 0 0 / .3)";
    document.body.appendChild(toast);
  }
  toast.textContent = message;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast?.remove(), 3200);
}

function escapeHTML(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}
