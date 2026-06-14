// Baked data for the hero process animation — kapi end to end, as a six-stage
// "show": Read → Pre-process → Pseudo-translate → Leverage → Translate (ja) →
// Merge. STATIC RenderDoc frames so the hero pulls ZERO wasm on page load (the
// engine only boots when the reader opens the modal); the structure mirrors a
// real pptx slide extraction — an Acme pitch-deck financials slide (title +
// bullets). Each frame bakes the exact text and overlays for its stage so
// FormatPreview animates the source → target deck progression: terms decode in
// when annotated, confidential figures stay redacted (a marker censor bar) all
// the way through, translation-memory hits roll in with a "from memory" highlight,
// and the pseudo + Japanese target cards slide in onto the deck.

import type { RenderDoc, RenderLine } from "@neokapi/ui-primitives/preview";
import type { OverlayView } from "@neokapi/ui-primitives/preview";

export type StageKey = "read" | "preprocess" | "pseudo" | "leverage" | "translate" | "merge";

export interface ProcessStage {
  key: StageKey;
  /** Full stage name (chrome + caption). */
  label: string;
  /** Compact stepper label. */
  short: string;
  /** One-line description shown under the stage. */
  caption: string;
}

export const STAGES: ProcessStage[] = [
  {
    key: "read",
    label: "Read",
    short: "Read",
    caption: "Parse any of 50+ formats into one faithful content model.",
  },
  {
    key: "preprocess",
    label: "Pre-process",
    short: "Prep",
    caption: "Redact sensitive spans, annotate terms, segment sentences.",
  },
  {
    key: "pseudo",
    label: "Pseudo-translate",
    short: "Pseudo",
    caption: "Expand and accent the text to stress-test layout before real translation.",
  },
  {
    key: "leverage",
    label: "Leverage",
    short: "Leverage",
    caption: "Reuse exact and fuzzy matches from translation memory.",
  },
  {
    key: "translate",
    label: "Translate · 日本語",
    short: "日本語",
    caption: "Translate the remainder — terminology and inline tags preserved.",
  },
  {
    key: "merge",
    label: "Merge",
    short: "Merge",
    caption: "Write the translation back into the original file — byte-faithful.",
  },
];

// A few of the formats kapi reads, surfaced as chips during the Read stage.
export const READ_FORMATS = [
  "PPTX",
  "DOCX",
  "XLSX",
  "XLIFF",
  "JSON",
  "YAML",
  "HTML",
  "Markdown",
  "PO",
  "CSV",
  "ARB",
  "RESX",
];

const SLIDE = "ppt/slides/slide1.xml";
const DUMMY_RANGE = { startRun: 0, startOffset: 0, endRun: 0, endOffset: 0 };

// Confidential pitch-deck figures — the canonical "redact before translation"
// case. Kept verbatim across stages so the redaction overlay can locate them: the
// figures are masked from pre-process through translation (never sent to the
// translator) and restored locally at merge.
const REVENUE = "$48M";
const BURN = "$1.2M";

// Source line text (English), keyed by id so frames line up across stages.
const SRC = {
  t: "Acme — Series B",
  b1: `FY27 revenue: ${REVENUE}`,
  b2: `Net burn: ${BURN}/mo`,
  b3: "Acme owns the category.",
};

// Pseudo-translate (accented + lightly expanded) — visual flavour only. The
// figures are left verbatim (unaccented) so the redaction overlay still matches.
const PSEUDO = {
  t: "Àçmé — Sérîés B",
  b1: `FÝ27 révénüé: ${REVENUE}`,
  b2: `Nét bürn: ${BURN}/mö`,
  b3: "Àçmé öwns thé çàtégöry.",
};

// Japanese translations. The figures stay masked through translation, restored at merge.
const JA = {
  t: "Acme — シリーズ B",
  b1: `FY27 売上：${REVENUE}`,
  b2: `純バーン：${BURN}/月`,
  b3: "Acme が市場を制する。",
};

// Redaction overlays covering the confidential figures — painted as censor bars.
const redactRevenue = (): OverlayView =>
  overlay("redaction", [span(REVENUE, { category: "Projected revenue" })]);
const redactBurn = (): OverlayView => overlay("redaction", [span(BURN, { category: "Burn rate" })]);
// A term annotation for the "Acme" brand name — decodes in when annotated.
const acmeTerm = (): OverlayView =>
  overlay("term", [span("Acme", { target: "Acme", domain: "brand" })]);
// A line-level marker: this line was filled from translation memory.
const fromMemory = (): OverlayView => overlay("tm", []);

function slide(title: RenderLine, bullets: RenderLine[]): RenderDoc {
  return { kind: "slides", format: "openxml", slides: [{ name: SLIDE, title, bullets }] };
}

function line(id: string, text: string, overlays?: OverlayView[]): RenderLine {
  return { id, text, role: id === "t" ? "title" : "bullet", ...(overlays ? { overlays } : {}) };
}

function span(text: string, props?: Record<string, string>) {
  return { range: DUMMY_RANGE, text, ...(props ? { props } : {}) };
}

function overlay(type: OverlayView["type"], spans: OverlayView["spans"]): OverlayView {
  return { type, side: "source", spans };
}

// ── Frames, one per stage ────────────────────────────────────────────────────

const readFrame = slide(line("t", SRC.t), [
  line("b1", SRC.b1),
  line("b2", SRC.b2),
  line("b3", SRC.b3),
]);

// Pre-process: "Acme" tagged as a term (decodes in) + the revenue and burn
// figures redacted (censor bars).
const preprocessFrame = slide(line("t", SRC.t, [acmeTerm()]), [
  line("b1", SRC.b1, [redactRevenue()]),
  line("b2", SRC.b2, [redactBurn()]),
  line("b3", SRC.b3, [acmeTerm()]),
]);

// Pseudo-translate: target card slides in; figures stay redacted.
const pseudoFrame = slide(line("t", PSEUDO.t), [
  line("b1", PSEUDO.b1, [redactRevenue()]),
  line("b2", PSEUDO.b2, [redactBurn()]),
  line("b3", PSEUDO.b3),
]);

// Leverage: title + last bullet come back from TM (100% match) — they roll in with
// a "from memory" highlight; the financial lines await translation, still redacted.
const leverageFrame = slide(line("t", JA.t, [fromMemory()]), [
  line("b1", SRC.b1, [redactRevenue()]),
  line("b2", SRC.b2, [redactBurn()]),
  line("b3", JA.b3, [fromMemory()]),
]);

// Translate: the Japanese target card slides in; the figures remain redacted.
const translateFrame = slide(line("t", JA.t), [
  line("b1", JA.b1, [redactRevenue()]),
  line("b2", JA.b2, [redactBurn()]),
  line("b3", JA.b3),
]);

// Merge writes the localized deck back and restores the figures locally — they
// were masked only so they never reached the translator; the final deck is whole.
const mergeFrame = slide(line("t", JA.t), [
  line("b1", JA.b1),
  line("b2", JA.b2),
  line("b3", JA.b3),
]);

export interface StageFrame {
  doc: RenderDoc;
  /** FormatPreview transition into this frame. */
  transition: "none" | "crossfade" | "typewriter" | "slot";
  /** Show overlay highlights for this frame. */
  annotations: boolean;
  /** Slide a fresh card in onto the deck (the target deck arriving). */
  slideIn?: boolean;
  /**
   * Locale folder shown in the on-deck file label (en → qps → ja) — the source
   * deck becomes the pseudo deck becomes the Japanese target deck.
   */
  locale: "en" | "qps" | "ja";
  /** Optional corner badge (stage-specific chrome). */
  badge?: string;
}

export const FRAMES: Record<StageKey, StageFrame> = {
  read: { doc: readFrame, transition: "crossfade", annotations: false, locale: "en" },
  // Terms decode in + figures redacted; annotations on so both render.
  preprocess: { doc: preprocessFrame, transition: "crossfade", annotations: true, locale: "en" },
  // The pseudo target deck slides in; figures stay redacted (annotations on).
  pseudo: { doc: pseudoFrame, transition: "none", annotations: true, slideIn: true, locale: "qps" },
  // TM hits roll in (slot) with a "from memory" highlight; figures redacted.
  leverage: {
    doc: leverageFrame,
    transition: "slot",
    annotations: true,
    badge: "2 / 4 from memory · 100%",
    locale: "ja",
  },
  // The Japanese target deck slides in; figures still redacted (annotations on).
  translate: {
    doc: translateFrame,
    transition: "none",
    annotations: true,
    slideIn: true,
    locale: "ja",
  },
  // Merge restores the figures — the crossfade reveals the real numbers again.
  merge: { doc: mergeFrame, transition: "crossfade", annotations: false, locale: "ja" },
};

/** The deck file name shown (locale-prefixed) on the card. */
export const HERO_FILENAME = "deck.pptx";
