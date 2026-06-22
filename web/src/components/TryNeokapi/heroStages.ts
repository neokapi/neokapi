// Baked data for the hero process animation — kapi end to end, as a six-stage
// "show": Read → Pre-process → Pseudo-translate → Leverage → Translate (ja) →
// Merge. STATIC RenderDoc frames so the hero pulls ZERO wasm on page load (the
// engine only boots when the reader opens the modal); the structure mirrors a
// real pptx slide extraction — an Acme pitch-deck financials slide (title +
// bullets). Each frame bakes the exact text and overlays for its stage so
// FormatPreview animates the source → target progression in place on one card:
// terms decode in when annotated, confidential figures stay redacted (a marker
// censor bar) all the way through, the text rolls to the accented pseudo form and
// then to Japanese, and translation-memory hits roll in with a "from memory"
// highlight. The card tints by locale (en → qps → ja) to track the progression.

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

// One show, the journey in miniature: get the source right (read · check), then
// get it everywhere (preview · reuse · translate · merge). Captions in the
// builder voice; no hardcoded counts, no "faithful" — "byte-for-byte".
export const STAGES: ProcessStage[] = [
  {
    key: "read",
    label: "Read",
    short: "Read",
    caption: "Parse any format into one content model — structure and styles intact.",
  },
  {
    key: "preprocess",
    label: "Get it right",
    short: "Check",
    caption: "Check the source — terms and brand on point, sensitive spans protected.",
  },
  {
    key: "pseudo",
    label: "Pseudo-translate",
    short: "Preview",
    caption: "Preview every language before you ship — catch layout breaks and hardcoded strings early.",
  },
  {
    key: "leverage",
    label: "Leverage",
    short: "Reuse",
    caption: "Only re-translate what changed — reuse what you've already translated.",
  },
  {
    key: "translate",
    label: "Translate · 日本語",
    short: "日本語",
    caption:
      "Translate the rest with AI, then a quick human check — terms and inline tags intact.",
  },
  {
    key: "merge",
    label: "Merge",
    short: "Merge",
    caption: "Write it back into the original file — byte-for-byte.",
  },
];

// A few of the formats kapi reads, surfaced as chips during the Read stage.
export const READ_FORMATS = ["PPTX", "DOCX", "XLSX", "XLIFF", "JSON", "Markdown"];

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

// Pseudo-translate exactly like `kapi pseudo-translate`: the accent map +
// ▒ shade markers from core/tools/pseudo.go, so the hero matches the real tool
// (every ASCII letter is accented; digits/punctuation pass through).
const ACCENT: Record<string, string> = {
  a: "à",
  b: "ƃ",
  c: "ç",
  d: "đ",
  e: "é",
  f: "ƒ",
  g: "ĝ",
  h: "ĥ",
  i: "î",
  j: "ĵ",
  k: "ķ",
  l: "ļ",
  m: "ḿ",
  n: "ñ",
  o: "ö",
  p: "þ",
  q: "ǫ",
  r: "ŕ",
  s: "š",
  t: "ţ",
  u: "ü",
  v: "ṽ",
  w: "ŵ",
  x: "ẋ",
  y: "ý",
  z: "ž",
  A: "À",
  B: "Ƃ",
  C: "Ç",
  D: "Đ",
  E: "É",
  F: "Ƒ",
  G: "Ĝ",
  H: "Ĥ",
  I: "Î",
  J: "Ĵ",
  K: "Ķ",
  L: "Ļ",
  M: "Ḿ",
  N: "Ñ",
  O: "Ö",
  P: "Þ",
  Q: "Ǫ",
  R: "Ŕ",
  S: "Š",
  T: "Ţ",
  U: "Ü",
  V: "Ṽ",
  W: "Ŵ",
  X: "Ẋ",
  Y: "Ý",
  Z: "Ž",
};
const accent = (s: string): string => s.replace(/[A-Za-z]/g, (c) => ACCENT[c] ?? c);
// The brand name is a protected term: pseudo-translation leaves it verbatim, so it
// stays "Acme" (never accented) even as the surrounding text is pseudo-translated.
const BRAND = "Acme";
const accentKeepBrand = (s: string): string => s.split(BRAND).map(accent).join(BRAND);
const pseudo = (s: string): string => `▒ ${accentKeepBrand(s)} ▒`;

// Pseudo-translate (real accent map + shade markers). The figures are accented
// like everything else; the redaction overlay locates the accented form.
const PSEUDO = {
  t: pseudo(SRC.t),
  b1: pseudo(SRC.b1),
  b2: pseudo(SRC.b2),
  b3: pseudo(SRC.b3),
};

// Japanese translations. The figures stay masked through translation, restored at merge.
const JA = {
  t: "Acme — シリーズ B",
  b1: `FY27 売上：${REVENUE}`,
  b2: `純バーン：${BURN}/月`,
  b3: "Acme が市場を制する。",
};

// Redaction overlays covering the confidential figures — painted as censor bars.
// `figure` must be the exact substring as it appears in the line (accented under
// pseudo), so the overlay can locate it.
const redact = (figure: string, category: string): OverlayView =>
  overlay("redaction", [span(figure, { category })]);
// A term annotation for the "Acme" brand name — decodes in when annotated, and
// stays marked through pseudo/translation since the brand term is preserved verbatim.
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
  line("b1", SRC.b1, [redact(REVENUE, "Projected revenue")]),
  line("b2", SRC.b2, [redact(BURN, "Burn rate")]),
  line("b3", SRC.b3, [acmeTerm()]),
]);

// Pseudo-translate: the text rolls in place from source to accented; the brand
// term stays marked and the figures stay redacted (accented form) throughout.
const pseudoFrame = slide(line("t", PSEUDO.t, [acmeTerm()]), [
  line("b1", PSEUDO.b1, [redact(accent(REVENUE), "Projected revenue")]),
  line("b2", PSEUDO.b2, [redact(accent(BURN), "Burn rate")]),
  line("b3", PSEUDO.b3, [acmeTerm()]),
]);

// Leverage: title + last bullet come back from TM (100% match) — they roll in with
// a "from memory" highlight; the financial lines await translation, still redacted.
const leverageFrame = slide(line("t", JA.t, [fromMemory()]), [
  line("b1", SRC.b1, [redact(REVENUE, "Projected revenue")]),
  line("b2", SRC.b2, [redact(BURN, "Burn rate")]),
  line("b3", JA.b3, [fromMemory()]),
]);

// Translate: the Japanese target card slides in; the figures remain redacted.
const translateFrame = slide(line("t", JA.t), [
  line("b1", JA.b1, [redact(REVENUE, "Projected revenue")]),
  line("b2", JA.b2, [redact(BURN, "Burn rate")]),
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
  /** FormatPreview transition into this frame (animated in place). */
  transition: "none" | "crossfade" | "typewriter" | "slot";
  /** Show overlay highlights for this frame. */
  annotations: boolean;
  /**
   * Locale folder shown in the on-deck file label (en → qps → ja) — and the card
   * tint: the source deck becomes the pseudo deck becomes the Japanese target deck.
   */
  locale: "en" | "qps" | "ja";
  /** Optional corner badge (stage-specific chrome). */
  badge?: string;
}

export const FRAMES: Record<StageKey, StageFrame> = {
  read: { doc: readFrame, transition: "crossfade", annotations: false, locale: "en" },
  // Terms decode in + figures redacted; annotations on so both render.
  preprocess: { doc: preprocessFrame, transition: "crossfade", annotations: true, locale: "en" },
  // The text rolls in place to the accented pseudo form; figures stay redacted.
  pseudo: { doc: pseudoFrame, transition: "slot", annotations: true, locale: "qps" },
  // TM hits roll in (slot) with a "from memory" highlight; figures redacted.
  leverage: {
    doc: leverageFrame,
    transition: "slot",
    annotations: true,
    badge: "2 / 4 from memory · 100%",
    locale: "ja",
  },
  // The remaining lines roll in place to Japanese; figures still redacted.
  translate: {
    doc: translateFrame,
    transition: "slot",
    annotations: true,
    locale: "ja",
    badge: "AI or human · reviewed",
  },
  // Merge restores the figures — the crossfade reveals the real numbers again.
  merge: { doc: mergeFrame, transition: "crossfade", annotations: false, locale: "ja" },
};

/** The deck file name shown (locale-prefixed) on the card. */
export const HERO_FILENAME = "deck.pptx";
