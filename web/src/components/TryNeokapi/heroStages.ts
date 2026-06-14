// Baked data for the hero process animation — kapi end to end, as a six-stage
// "show": Read → Pre-process → Pseudo-translate → Leverage → Translate (ja) →
// Merge. STATIC RenderDoc frames so the hero pulls ZERO wasm on page load (the
// engine only boots when the reader opens the modal); the structure mirrors a
// real pptx slide extraction (one slide, a title + bullets). Each frame bakes
// the exact text shown at that stage so FormatPreview's slot-text roll / crossfade
// animates the source → pseudo → Japanese progression line by line.

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

// A secret that must never reach an MT/LLM — an API key is the canonical
// "redact before translation" case. Kept verbatim across stages so the redaction
// overlay can locate it and so it can be restored once translation is done.
const SECRET = "sk_live_5fK2aR9wQ8";

// Source line text (English), keyed by id so frames line up for the diff.
const SRC = {
  t: "Welcome to Acme",
  b1: `Your API key: ${SECRET}`,
  b2: "Acme makes every quarter count.",
  b3: "Sign up for Acme today.",
};

// Pseudo-translate (accented + lightly expanded) — visual flavour only. The
// secret is left verbatim (unaccented) so the redaction overlay still matches it.
const PSEUDO = {
  t: "Ŵélçömé tö Àçmé",
  b1: `Ýöür ÀPÌ kéy: ${SECRET}`,
  b2: "Àçmé màkés évéry qüàrtér çöünt.",
  b3: "Sîgn üp för Àçmé tödày.",
};

// Japanese translations. The secret is restored verbatim after translation.
const JA = {
  t: "Acme へようこそ",
  b1: `API キー：${SECRET}`,
  b2: "Acme は四半期ごとを大切にします。",
  b3: "今すぐ Acme に登録しましょう。",
};

// The redaction overlay covering the secret — painted as a marker censor bar.
const redactSecret = (): OverlayView =>
  overlay("redaction", [span(SECRET, { category: "API secret" })]);

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

// Pre-process: the API key is redacted (censor bar) + "Acme" tagged as a term.
const preprocessFrame = slide(
  line("t", SRC.t, [overlay("term", [span("Acme", { target: "Acme", domain: "brand" })])]),
  [
    line("b1", SRC.b1, [redactSecret()]),
    line("b2", SRC.b2, [overlay("term", [span("Acme", { target: "Acme", domain: "brand" })])]),
    line("b3", SRC.b3, [overlay("term", [span("Acme", { target: "Acme", domain: "brand" })])]),
  ],
);

// Pseudo-translate: the secret stays redacted while the rest is pseudo-translated.
const pseudoFrame = slide(line("t", PSEUDO.t), [
  line("b1", PSEUDO.b1, [redactSecret()]),
  line("b2", PSEUDO.b2),
  line("b3", PSEUDO.b3),
]);

// Leverage: title + second bullet come back from TM (100% match); the rest still
// awaits translation (shown in source), the secret still redacted.
const leverageFrame = slide(line("t", JA.t), [
  line("b1", SRC.b1, [redactSecret()]),
  line("b2", JA.b2),
  line("b3", SRC.b3),
]);

const translateFrame = slide(line("t", JA.t), [
  line("b1", JA.b1),
  line("b2", JA.b2),
  line("b3", JA.b3),
]);

const mergeFrame = translateFrame;

export interface StageFrame {
  doc: RenderDoc;
  /** FormatPreview transition into this frame. */
  transition: "none" | "crossfade" | "typewriter" | "slot";
  /** Show overlay highlights for this frame. */
  annotations: boolean;
  /** Optional corner badge (stage-specific chrome). */
  badge?: string;
}

export const FRAMES: Record<StageKey, StageFrame> = {
  read: { doc: readFrame, transition: "crossfade", annotations: false },
  preprocess: { doc: preprocessFrame, transition: "crossfade", annotations: true },
  // annotations on so the redaction censor bar renders; the redacted line opts out
  // of the slot roll (it would briefly expose the secret), the rest still roll.
  pseudo: { doc: pseudoFrame, transition: "slot", annotations: true },
  leverage: {
    doc: leverageFrame,
    transition: "crossfade",
    annotations: true,
    badge: "2 / 4 from memory · 100%",
  },
  translate: { doc: translateFrame, transition: "slot", annotations: false },
  merge: { doc: mergeFrame, transition: "crossfade", annotations: false, badge: "deck.pptx" },
};

/** The file label shown in the stage chrome. */
export const HERO_FILENAME = "deck.pptx";
