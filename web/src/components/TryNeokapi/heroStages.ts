// Baked data for the hero process animation — the kapi way, as a six-stage
// "show" of one command: Declare → Reconcile → Produce → Converge → Review →
// Shipped. This is the anatomy of `kapi up`: you declare the desired state in a
// recipe (content, languages, ship gate) and one verb reconciles reality to it —
// reusing what's already translated, producing the rest with your AI, checking
// as it goes, looping to the gate, and parking only what a human must judge. The
// toil collapses into the loop; you touch what matters and nothing else.
//
// STATIC RenderDoc frames so the hero pulls ZERO wasm on page load (the engine
// only boots when the reader opens the modal); the structure mirrors a real pptx
// slide extraction — an Acme pitch-deck financials slide (title + bullets). Each
// frame bakes the exact text and overlays for its stage so FormatPreview animates
// the source → converged progression in place on one card: terms decode in when
// checked, confidential figures stay redacted (a censor bar) from reconcile
// through convergence — never sent to a translator — and are restored locally
// only when the file ships; translation-memory reuse rolls in with a "from
// memory" highlight beside the AI-produced lines. The card tints by locale
// (en → ja) to track the progression from source of truth to shipped target.

import type { RenderDoc, RenderLine } from "@neokapi/ui-primitives/preview";
import type { OverlayView } from "@neokapi/ui-primitives/preview";

export type StageKey = "declare" | "reconcile" | "produce" | "converge" | "review" | "shipped";

export interface ProcessStage {
  key: StageKey;
  /** Full stage name (chrome + caption). */
  label: string;
  /** Compact stepper label. */
  short: string;
  /** One-line description shown under the stage. */
  caption: string;
}

// One verb, the loop in miniature: declare what "done" means, then let `kapi up`
// reconcile reality to it. Captions in the builder voice — no hardcoded counts,
// no "faithful" ("byte-for-byte"), no CAT-tool jargon ("leverage", "merge").
export const STAGES: ProcessStage[] = [
  {
    key: "declare",
    label: "Declare",
    short: "Recipe",
    caption:
      "The recipe is the desired state — your content, your languages, the bar for shipped.",
  },
  {
    key: "reconcile",
    label: "kapi up",
    short: "Reconcile",
    caption:
      "One verb reconciles reality to the recipe — checks the source, protects what's sensitive.",
  },
  {
    key: "produce",
    label: "Produce",
    short: "Produce",
    caption:
      "Every language in one pass — reuse what's already translated, produce the rest with your AI.",
  },
  {
    key: "converge",
    label: "Converge",
    short: "Gate",
    caption:
      "Checks run in the loop; each language climbs toward its ship gate — no spreadsheet.",
  },
  {
    key: "review",
    label: "Review",
    short: "Review",
    caption:
      "You review only what a human must judge — the rest has already cleared its gate.",
  },
  {
    key: "shipped",
    label: "Shipped",
    short: "Shipped",
    caption:
      "Converged: every file written back byte-for-byte, sensitive figures restored locally.",
  },
];

// A few of the formats kapi reads, surfaced as chips during the Declare stage —
// the recipe points at your real files, whatever they are.
export const READ_FORMATS = ["PPTX", "DOCX", "XLSX", "XLIFF", "JSON", "Markdown"];

const SLIDE = "ppt/slides/slide1.xml";
const DUMMY_RANGE = { startRun: 0, startOffset: 0, endRun: 0, endOffset: 0 };

// Confidential pitch-deck figures — the canonical "redact before translation"
// case. Kept verbatim across stages so the redaction overlay can locate them: the
// figures are masked from reconcile through convergence (never sent to a
// translator or a hosted model) and restored locally only when the file ships.
const REVENUE = "$48M";
const BURN = "$1.2M";

// Source line text (English), keyed by id so frames line up across stages.
const SRC = {
  t: "Acme — Series B",
  b1: `FY27 revenue: ${REVENUE}`,
  b2: `Net burn: ${BURN}/mo`,
  b3: "Acme owns the category.",
};

// Japanese translations. The figures stay masked through translation, restored
// locally at ship time.
const JA = {
  t: "Acme — シリーズ B",
  b1: `FY27 売上：${REVENUE}`,
  b2: `純バーン：${BURN}/月`,
  b3: "Acme が市場を制する。",
};

// Redaction overlays covering the confidential figures — painted as censor bars.
// `figure` must be the exact substring as it appears in the line so the overlay
// can locate it.
const redact = (figure: string, category: string): OverlayView =>
  overlay("redaction", [span(figure, { category })]);
// A term annotation for the "Acme" brand name — decodes in when the source is
// checked, and stays marked through production since the brand term is preserved
// verbatim across every language.
const acmeTerm = (): OverlayView =>
  overlay("term", [span("Acme", { target: "Acme", domain: "brand" })]);
// A line-level marker: this line was reused from translation memory (not re-produced).
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

// Declare: the source deck is the desired state — plain, no work done yet. The
// format chips render alongside this stage (the recipe reads any format).
const declareFrame = slide(line("t", SRC.t), [
  line("b1", SRC.b1),
  line("b2", SRC.b2),
  line("b3", SRC.b3),
]);

// Reconcile: `kapi up` re-derives what needs doing and checks the source — "Acme"
// tagged as a term (decodes in), and the revenue and burn figures redacted
// (censor bars) so they never leave the machine.
const reconcileFrame = slide(line("t", SRC.t, [acmeTerm()]), [
  line("b1", SRC.b1, [redact(REVENUE, "Projected revenue")]),
  line("b2", SRC.b2, [redact(BURN, "Burn rate")]),
  line("b3", SRC.b3, [acmeTerm()]),
]);

// Produce: one pass, every language. The title and the last bullet come back
// from translation memory (100% match, "from memory" highlight); the financial
// lines are produced with AI — still redacted, so the figures never reach the
// model. Mixed provenance in a single pass.
const produceFrame = slide(line("t", JA.t, [fromMemory()]), [
  line("b1", JA.b1, [redact(REVENUE, "Projected revenue")]),
  line("b2", JA.b2, [redact(BURN, "Burn rate")]),
  line("b3", JA.b3, [fromMemory()]),
]);

// Converge: the checks pass in the loop — terms and inline tags intact — and each
// language climbs its ladder. The brand term stays marked; the figures stay
// redacted (not shipped yet). The badge carries the standing.
const convergeFrame = slide(line("t", JA.t, [acmeTerm()]), [
  line("b1", JA.b1, [redact(REVENUE, "Projected revenue")]),
  line("b2", JA.b2, [redact(BURN, "Burn rate")]),
  line("b3", JA.b3, [acmeTerm()]),
]);

// Review: everything that can clear its gate unaided already has; one brand claim
// is parked for a human — it rolls in from the review queue, the only line you
// actually touch.
const reviewFrame = slide(line("t", JA.t), [
  line("b1", JA.b1, [redact(REVENUE, "Projected revenue")]),
  line("b2", JA.b2, [redact(BURN, "Burn rate")]),
  line("b3", JA.b3, [fromMemory()]),
]);

// Shipped: the gate is green. The localized deck is written back into the
// original file and the figures are restored locally — masked only so they never
// reached a translator; the shipped deck is whole.
const shippedFrame = slide(line("t", JA.t), [
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
   * Locale folder shown in the on-deck file label (en → ja) — and the card tint:
   * the source deck (the desired state) becomes the shipped Japanese target.
   */
  locale: "en" | "qps" | "ja";
  /** Optional corner badge (stage-specific chrome). */
  badge?: string;
}

export const FRAMES: Record<StageKey, StageFrame> = {
  // The desired state: the source deck, with the ship gate as the target.
  declare: {
    doc: declareFrame,
    transition: "crossfade",
    annotations: false,
    locale: "en",
    badge: "gate · translated 100 · reviewed",
  },
  // Terms decode in + figures redacted; annotations on so both render.
  reconcile: { doc: reconcileFrame, transition: "crossfade", annotations: true, locale: "en" },
  // TM reuse + AI production roll in together (slot) in one pass; figures redacted.
  produce: {
    doc: produceFrame,
    transition: "slot",
    annotations: true,
    locale: "ja",
    badge: "reuse + your AI · one pass",
  },
  // Checks pass in the loop; the standing climbs. The ladder rides the badge.
  converge: {
    doc: convergeFrame,
    transition: "crossfade",
    annotations: true,
    locale: "ja",
    badge: "de ✓ · ja ✓ · fr parked",
  },
  // One parked claim rolls in for a human; everything else already cleared.
  review: {
    doc: reviewFrame,
    transition: "slot",
    annotations: true,
    locale: "ja",
    badge: "human · only where it matters",
  },
  // Ship: the figures are restored — the crossfade reveals the real numbers again.
  shipped: {
    doc: shippedFrame,
    transition: "crossfade",
    annotations: false,
    locale: "ja",
    badge: "converged · byte-for-byte",
  },
};

/** The deck file name shown (locale-prefixed) on the card. */
export const HERO_FILENAME = "deck.pptx";
