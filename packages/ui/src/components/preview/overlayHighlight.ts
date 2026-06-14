// overlayHighlight — map a block's stand-off overlays onto a flat text string,
// producing color-coded, tooltipped highlight segments for FormatPreview.
//
// Overlays anchor to run-index ranges, but the renderer works with the block's
// concatenated literal text (RenderLine.text / targets[locale]). Each
// OverlaySpanView already carries the engine-extracted `text` it covers, so we
// locate spans by substring match over the rendered text — robust across the
// run↔text projection — and fall back to nothing when a span's text can't be
// found (e.g. it covered only inline markup). This mirrors the contract:
// "locate spans by range over the runs, or fall back to matching span.text".

import type { OverlaySpan, OverlayView } from "./types";

// ── Overlay type → accent ────────────────────────────────────────────────────
//
// The palette keys on the ACTUAL overlay types the engine emits
// (`labInspectAnnotated`): `term` (vocabulary), and `qa` — where a `qa` overlay
// whose `props.category` is `"brand-vocabulary"` is a brand violation, and every
// other `qa` overlay is a rule-based check (double spaces, doubled words). So a
// span's accent is decided by its overlay type AND its span props, not the type
// alone: term = violet (vocabulary), brand-vocabulary = pink (a brand QA), other
// qa = amber. Other model overlay types (entity/segmentation/alignment) keep
// stable accents in case a future annotator emits them.

export interface OverlayStyle {
  /** Tailwind classes applied to the highlight <mark>. */
  className: string;
  /** A short human label for the overlay type (tooltip header). */
  label: string;
}

// Brand violations ride on the `qa` overlay type and are distinguished by this
// span-prop category. Keep in sync with the wasm annotator (lab_annotate.go).
const BRAND_CATEGORY = "brand-vocabulary";

// Effective-accent keys. The renderer never sees these directly — they are the
// resolved identity (type + category) that `overlayStyle` maps to an accent.
const OVERLAY_STYLES: Record<string, OverlayStyle> = {
  // Terminology / vocabulary matches from the termbase.
  term: { className: "bg-violet-500/20 text-violet-700 dark:text-violet-300", label: "Vocabulary" },
  terms: {
    className: "bg-violet-500/20 text-violet-700 dark:text-violet-300",
    label: "Vocabulary",
  },
  // Brand-vocabulary violations (a `qa` overlay with category=brand-vocabulary).
  [BRAND_CATEGORY]: {
    className: "bg-pink-500/20 text-pink-700 dark:text-pink-300",
    label: "Brand",
  },
  // Rule-based QA findings (double spaces, doubled words, …).
  qa: { className: "bg-amber-500/25 text-amber-700 dark:text-amber-300", label: "QA" },
  "qa-check": {
    className: "bg-amber-500/25 text-amber-700 dark:text-amber-300",
    label: "QA check",
  },
  // Other model overlay types (not currently produced, but kept stable).
  entity: { className: "bg-sky-500/20 text-sky-700 dark:text-sky-300", label: "Entity" },
  entities: { className: "bg-sky-500/20 text-sky-700 dark:text-sky-300", label: "Entity" },
  // Sensitive spans removed before translation. The className is unused — the
  // renderer paints a marker censor bar for redaction (see FormatPreview) — but a
  // style entry keeps the label/tooltip path consistent.
  redaction: { className: "", label: "Redacted" },
  segmentation: {
    className: "bg-slate-400/20 text-slate-700 dark:text-slate-300",
    label: "Segment",
  },
  alignment: { className: "bg-teal-500/20 text-teal-700 dark:text-teal-300", label: "Alignment" },
};

const DEFAULT_STYLE: OverlayStyle = {
  className: "bg-emerald-500/20 text-emerald-700 dark:text-emerald-300",
  label: "Annotation",
};

/**
 * The effective accent key for an overlay span. Brand violations ride on the
 * `qa` overlay type, so a `qa` span carrying `props.category="brand-vocabulary"`
 * resolves to the brand accent; everything else resolves on its overlay type.
 */
function effectiveKey(type: string, span?: OverlaySpan): string {
  if ((type === "qa" || type === "qa-check") && span?.props?.category === BRAND_CATEGORY) {
    return BRAND_CATEGORY;
  }
  return type;
}

/**
 * Resolve the accent + label for an overlay span. Pass the span so a brand
 * violation (a `qa` overlay with category=brand-vocabulary) gets the brand
 * accent rather than the generic QA accent. Unknown keys fall back to emerald.
 */
export function overlayStyle(type: string, span?: OverlaySpan): OverlayStyle {
  const key = effectiveKey(type, span);
  return OVERLAY_STYLES[key] ?? { ...DEFAULT_STYLE, label: titleCase(key) };
}

function titleCase(s: string): string {
  return s
    .replace(/[-_]/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .trim();
}

// ── Span resolution ──────────────────────────────────────────────────────────

/** A located highlight: a [start,end) char range over the rendered text. */
export interface ResolvedSpan {
  start: number;
  end: number;
  type: string;
  style: OverlayStyle;
  span: OverlaySpan;
  /** A one-line tooltip describing the overlay (type + props/text). */
  tooltip: string;
}

// The props worth surfacing in the tooltip, per overlay identity — chosen so a
// reader sees the actionable fact, not the raw prop bag. Terms show their target
// translation (+ domain); brand violations show the suggested replacement (or
// the human message); other QA shows its message. Anything not listed falls back
// to a compact key:value join.
const TOOLTIP_PROPS: Record<string, string[]> = {
  term: ["target", "domain"],
  terms: ["target", "domain"],
  [BRAND_CATEGORY]: ["replacement", "message"],
  qa: ["message"],
  "qa-check": ["message"],
};

/** Build the tooltip line for an overlay span (type + the useful prop). */
function tooltipFor(type: string, span: OverlaySpan): string {
  const key = effectiveKey(type, span);
  const parts: string[] = [overlayStyle(type, span).label];
  const props = span.props;
  if (props) {
    const wanted = TOOLTIP_PROPS[key];
    const picked = wanted
      ? wanted.filter((k) => props[k] && props[k] !== "").map((k) => `${k}: ${props[k]}`)
      : Object.entries(props)
          .filter(([, v]) => v !== "")
          .map(([k, v]) => `${k}: ${v}`);
    if (picked.length > 0) parts.push(picked.join(" · "));
    else if (span.text) parts.push(`“${span.text}”`);
  } else if (span.text) {
    parts.push(`“${span.text}”`);
  }
  return parts.join(" — ");
}

/**
 * Resolve every overlay span for the active side into char ranges over `text`,
 * sorted by start. Spans whose text can't be located are dropped. Overlapping
 * spans are kept; the renderer flattens them (innermost-wins) below.
 *
 * @param overlays  the block's overlays (already filtered or not)
 * @param side      the active side: "source" or a target locale key
 * @param text      the rendered text for that side
 * @param filter    optional set of overlay types to include (undefined = all)
 */
export function resolveOverlaySpans(
  overlays: OverlayView[] | undefined,
  side: string,
  text: string,
  filter?: Set<string>,
): ResolvedSpan[] {
  if (!overlays || !text) return [];
  const out: ResolvedSpan[] = [];
  // Track a search cursor per (overlay, sequence) so repeated identical span
  // texts within one overlay map to successive occurrences, not all to the first.
  for (const ov of overlays) {
    if (ov.side !== side) continue;
    // "tm" is a line-level marker (TM leverage), not a span highlight — the
    // renderer reads it off the line directly and styles the whole line.
    if (ov.type === "tm") continue;
    if (filter && !filter.has(ov.type)) continue;
    let cursor = 0;
    for (const span of ov.spans) {
      if (span.ignorable) continue;
      const needle = span.text ?? "";
      if (!needle) continue;
      const idx = text.indexOf(needle, cursor);
      if (idx < 0) continue;
      out.push({
        start: idx,
        end: idx + needle.length,
        type: ov.type,
        // Style/tooltip key on the span too, so a brand-vocabulary qa span gets
        // the brand accent rather than the generic QA accent.
        style: overlayStyle(ov.type, span),
        span,
        tooltip: tooltipFor(ov.type, span),
      });
      cursor = idx + Math.max(1, needle.length);
    }
  }
  out.sort((a, b) => a.start - b.start || b.end - a.end);
  return out;
}

// ── Segment flattening ───────────────────────────────────────────────────────

/** A flat, non-overlapping run of text either plain or carrying one overlay. */
export interface TextSegment {
  text: string;
  /** The covering overlay (innermost wins on overlap), or undefined when plain. */
  overlay?: ResolvedSpan;
}

/**
 * Flatten `text` + resolved spans into a non-overlapping sequence of segments.
 * On overlap the shorter (innermost) span wins so a term inside an entity still
 * shows. This keeps rendering a simple map over segments.
 */
export function segmentText(text: string, spans: ResolvedSpan[]): TextSegment[] {
  if (spans.length === 0) return text ? [{ text }] : [];
  // For each char position, pick the smallest covering span (innermost wins).
  const owner: (ResolvedSpan | undefined)[] = Array.from({ length: text.length });
  for (const s of spans) {
    const width = s.end - s.start;
    for (let i = s.start; i < s.end && i < text.length; i++) {
      const cur = owner[i];
      if (!cur || s.end - s.start <= cur.end - cur.start) {
        // smaller-or-equal width wins (later equal-width span also wins, fine)
        if (!cur || width <= cur.end - cur.start) owner[i] = s;
      }
    }
  }
  const segs: TextSegment[] = [];
  let i = 0;
  while (i < text.length) {
    const cur = owner[i];
    let j = i + 1;
    while (j < text.length && owner[j] === cur) j++;
    segs.push(cur ? { text: text.slice(i, j), overlay: cur } : { text: text.slice(i, j) });
    i = j;
  }
  return segs;
}

/** The distinct overlay types present, for a legend/filter UI. */
export function overlayTypes(overlays: OverlayView[] | undefined): string[] {
  if (!overlays) return [];
  const seen = new Set<string>();
  const order: string[] = [];
  for (const ov of overlays) {
    if (!seen.has(ov.type)) {
      seen.add(ov.type);
      order.push(ov.type);
    }
  }
  return order;
}
