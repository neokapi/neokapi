// Scene data for the Motion "Content Loop" hero (HeroLoop). Zero wasm: this is
// baked content — the engine only boots when the reader opens the modal. The hero
// tells the engine's job as one continuous loop, threading the two interlocking
// cycles from the docs: an authoring loop (get the source right) feeds the
// localization loop (get it everywhere), which writes back and repeats.
//
// The visible ring is the localization loop — READ → PREP → RECYCLE → TRANSLATE →
// CHECK → SHIP → (repeat) — fed by authoring. One persistent slide (an Acme
// pitch deck) morphs through every stage on a single card: terms get marked,
// confidential figures get redacted before they travel, recycled segments snap in
// from memory, the rest fills with AI, a human green-lines it, and the file is
// written back byte-for-byte in every language. Each stage carries a role color
// that the loop node and the card share, so the eye links "this stage" to "this
// change".

/** Role accent for a stage — mirrors the docs diagram-kit palette (kdx). */
export type Role = "io" | "annotate" | "resource" | "translate" | "qa";

/** A line on the slide. Figures carry a redactable span; text lines can be
 *  marked as a protected term, recycled from memory, or human-approved. */
export type DocLine =
  | {
      id: "t" | "b3";
      kind: "text";
      text: string;
      /** Brand term "Acme" is highlighted as a protected term. */
      term?: boolean;
      /** Filled from translation memory (100% match). */
      memory?: boolean;
      /** Human-confirmed in review. */
      approved?: boolean;
      /** Rendered in the target language (affects subtle styling only). */
      target?: boolean;
    }
  | {
      id: "b1" | "b2";
      kind: "figure";
      pre: string;
      /** The confidential figure — masked by a redaction bar while `redacted`. */
      figure: string;
      post: string;
      redacted?: boolean;
      approved?: boolean;
      target?: boolean;
      /** Figures are never recycled from memory, but the field keeps the union
       *  uniform so a line can be inspected without narrowing first. */
      memory?: boolean;
    };

export interface Scene {
  key: "read" | "prep" | "recycle" | "translate" | "check" | "ship";
  /** Mono stage name shown on the active loop node and the stage label. */
  stage: string;
  /** One-line description, in the builder voice — no hardcoded counts. */
  caption: string;
  role: Role;
  /** Locale-prefixed file path on the card (en → ja). */
  file: string;
  lines: DocLine[];
}

// Source (English) and Japanese strings, kept parallel so lines morph in place.
const T_EN = "Acme — Series B";
const T_JA = "Acme — シリーズ B";
const B3_EN = "Acme owns the category.";
const B3_JA = "Acme が市場を制する。";
const REVENUE = "$48M";
const BURN = "$1.2M";

export const SCENES: Scene[] = [
  {
    key: "read",
    stage: "Read",
    caption: "Read any document — slides, docs, tables, JSON — into one content model.",
    role: "io",
    file: "en/deck.pptx",
    lines: [
      { id: "t", kind: "text", text: T_EN },
      { id: "b1", kind: "figure", pre: "FY27 revenue: ", figure: REVENUE, post: "" },
      { id: "b2", kind: "figure", pre: "Net burn: ", figure: BURN, post: "/mo" },
      { id: "b3", kind: "text", text: B3_EN },
    ],
  },
  {
    key: "prep",
    stage: "Prep",
    caption: "Lock terminology and brand voice; redact anything sensitive before it travels.",
    role: "annotate",
    file: "en/deck.pptx",
    lines: [
      { id: "t", kind: "text", text: T_EN, term: true },
      {
        id: "b1",
        kind: "figure",
        pre: "FY27 revenue: ",
        figure: REVENUE,
        post: "",
        redacted: true,
      },
      { id: "b2", kind: "figure", pre: "Net burn: ", figure: BURN, post: "/mo", redacted: true },
      { id: "b3", kind: "text", text: B3_EN, term: true },
    ],
  },
  {
    key: "recycle",
    stage: "Recycle",
    caption: "Snap in everything you've translated before — pay only for what changed.",
    role: "resource",
    file: "ja/deck.pptx",
    lines: [
      { id: "t", kind: "text", text: T_JA, memory: true, target: true },
      {
        id: "b1",
        kind: "figure",
        pre: "FY27 revenue: ",
        figure: REVENUE,
        post: "",
        redacted: true,
      },
      { id: "b2", kind: "figure", pre: "Net burn: ", figure: BURN, post: "/mo", redacted: true },
      { id: "b3", kind: "text", text: B3_JA, memory: true, target: true },
    ],
  },
  {
    key: "translate",
    stage: "Translate",
    caption: "Fill every remaining language with AI — inline tags and terminology intact.",
    role: "translate",
    file: "ja/deck.pptx",
    lines: [
      { id: "t", kind: "text", text: T_JA, target: true },
      {
        id: "b1",
        kind: "figure",
        pre: "FY27 売上: ",
        figure: REVENUE,
        post: "",
        redacted: true,
        target: true,
      },
      {
        id: "b2",
        kind: "figure",
        pre: "純バーン: ",
        figure: BURN,
        post: "/月",
        redacted: true,
        target: true,
      },
      { id: "b3", kind: "text", text: B3_JA, target: true },
    ],
  },
  {
    key: "check",
    stage: "Check",
    caption: "Red-line, green-line: a native speaker confirms tone once, and kapi remembers.",
    role: "qa",
    file: "ja/deck.pptx",
    lines: [
      { id: "t", kind: "text", text: T_JA, approved: true, target: true },
      {
        id: "b1",
        kind: "figure",
        pre: "FY27 売上: ",
        figure: REVENUE,
        post: "",
        redacted: true,
        approved: true,
        target: true,
      },
      {
        id: "b2",
        kind: "figure",
        pre: "純バーン: ",
        figure: BURN,
        post: "/月",
        redacted: true,
        target: true,
      },
      { id: "b3", kind: "text", text: B3_JA, approved: true, target: true },
    ],
  },
  {
    key: "ship",
    stage: "Ship",
    caption: "Write each file back byte-for-byte — then loop, in every language.",
    role: "io",
    file: "ja/deck.pptx",
    lines: [
      { id: "t", kind: "text", text: T_JA, target: true },
      { id: "b1", kind: "figure", pre: "FY27 売上: ", figure: REVENUE, post: "", target: true },
      { id: "b2", kind: "figure", pre: "純バーン: ", figure: BURN, post: "/月", target: true },
      { id: "b3", kind: "text", text: B3_JA, target: true },
    ],
  },
];

/** A few of the formats kapi reads, surfaced as chips during Read. */
export const READ_FORMATS = ["PPTX", "DOCX", "XLSX", "XLIFF", "JSON", "MD"];

/** Languages the deck ships in, fanned out during Ship. The active target (ja)
 *  leads; the rest stand in for "every language". */
export const SHIP_LANGS = ["ja", "fr", "de", "es", "zh", "pt", "ko"];

/** The protected brand term, left verbatim across every stage. */
export const BRAND = "Acme";
