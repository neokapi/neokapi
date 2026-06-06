// Mock content + simulated transforms for the "Try Neokapi" modal showcase.
//
// IMPORTANT: everything here is FAKED — plain in-browser JS over hardcoded
// strings, with NO wasm and NO real files. The showcase exists to give a
// visitor an instant, polished picture of what the engine does to the
// translatable text inside three very different documents (a PowerPoint slide,
// an Excel sheet, a Markdown doc). The REAL engine only runs on the separate
// "Download result" / "Try with your own files" paths (see RealProof / OwnFiles).
//
// The mock content deliberately mirrors the real downloadable samples
// (TRY_SAMPLES in @neokapi/kapi-playground) so the faked preview and the real
// proof tell the same story: the term "Acme" appears throughout, and the default
// demo turns it into "Globex".

export type DemoId = "search-replace" | "insights" | "pseudo";

/** A single translatable string in a mock document, with a stable id. */
export interface MockField {
  id: string;
  /** A human label for the cell/region (e.g. a spreadsheet cell ref). */
  ref?: string;
  text: string;
}

export type MockKind = "pptx" | "xlsx" | "md";

export interface MockDoc {
  kind: MockKind;
  /** File name echoed on the card chrome. */
  filename: string;
  /** Title shown on the card chrome. */
  title: string;
  fields: MockField[];
}

// ── The three mock documents ─────────────────────────────────────────────────

export const MOCK_DOCS: MockDoc[] = [
  {
    kind: "pptx",
    filename: "deck.pptx",
    title: "PowerPoint slide",
    fields: [
      { id: "p1", ref: "Title", text: "Welcome to Acme" },
      { id: "p2", ref: "Subtitle", text: "Acme makes every quarter count." },
      { id: "p3", ref: "Body", text: "Sign up for Acme today" },
      { id: "p4", ref: "Body", text: "Talk to the Acme team soon" },
    ],
  },
  {
    kind: "xlsx",
    filename: "report.xlsx",
    title: "Excel worksheet",
    fields: [
      { id: "x1", ref: "A1", text: "Acme quarterly revenue" },
      { id: "x2", ref: "B1", text: "Total revenue" },
      { id: "x3", ref: "A2", text: "Acme net profit" },
      { id: "x4", ref: "B2", text: "Net profit" },
      { id: "x5", ref: "A3", text: "Acme customer count" },
      { id: "x6", ref: "B3", text: "Active accounts" },
    ],
  },
  {
    kind: "md",
    filename: "guide.md",
    title: "Markdown doc",
    fields: [
      { id: "m1", ref: "H1", text: "Welcome to Acme" },
      {
        id: "m2",
        ref: "Para",
        text: "Acme helps teams ship faster. Pick your favorite color and get started.",
      },
      { id: "m3", ref: "List", text: "Sign up for Acme today" },
      { id: "m4", ref: "List", text: "Talk to the Acme team soon" },
    ],
  },
];

// ── Simulated transforms ─────────────────────────────────────────────────────
//
// Each transform returns a list of segments per field so the renderer can
// highlight only the parts that changed (a real find/replace touches spans, not
// whole strings).

export interface Segment {
  text: string;
  changed: boolean;
}

/** Split `text` into highlighted segments where `find` was replaced by `replace`. */
export function applyReplace(text: string, find: string, replace: string): Segment[] {
  if (!find) return [{ text, changed: false }];
  const out: Segment[] = [];
  // Case-insensitive literal match so "color" finds "Color" too; the demo's
  // defaults are case-stable, but this keeps the preview robust to user typing.
  const needle = find.toLowerCase();
  let i = 0;
  const lower = text.toLowerCase();
  while (i < text.length) {
    const at = lower.indexOf(needle, i);
    if (at === -1) {
      out.push({ text: text.slice(i), changed: false });
      break;
    }
    if (at > i) out.push({ text: text.slice(i, at), changed: false });
    out.push({ text: replace, changed: true });
    i = at + find.length;
  }
  return out.length ? out : [{ text, changed: false }];
}

/** Plain joined string for a transformed field (used for the real-proof note). */
export function plainReplace(text: string, find: string, replace: string): string {
  return applyReplace(text, find, replace)
    .map((s) => s.text)
    .join("");
}

// A small accent map for the pseudo-translate showcase — mirrors the spirit of
// the engine's pseudo-translate (accent letters, surface truncation) without the
// length-padding / bracketing, so the preview stays legible.
const ACCENT: Record<string, string> = {
  a: "á",
  e: "é",
  i: "í",
  o: "ó",
  u: "ú",
  A: "Á",
  E: "É",
  I: "Í",
  O: "Ó",
  U: "Ú",
  n: "ñ",
  c: "ç",
  s: "š",
  y: "ý",
};

export function applyPseudo(text: string): Segment[] {
  // Every accented character is a "change"; runs of unchanged chars coalesce.
  const out: Segment[] = [];
  let buf = "";
  let bufChanged = false;
  const flush = () => {
    if (buf) out.push({ text: buf, changed: bufChanged });
    buf = "";
  };
  for (const ch of text) {
    const mapped = ACCENT[ch];
    const changed = mapped !== undefined;
    if (changed !== bufChanged) {
      flush();
      bufChanged = changed;
    }
    buf += mapped ?? ch;
  }
  flush();
  return out;
}

// ── Insights (per-document counts, computed in JS from the mock fields) ───────

export interface DocInsights {
  blocks: number;
  words: number;
  characters: number;
}

function countWords(text: string): number {
  const t = text.trim();
  return t ? t.split(/\s+/).length : 0;
}

export function insightsFor(doc: MockDoc): DocInsights {
  let words = 0;
  let characters = 0;
  for (const f of doc.fields) {
    words += countWords(f.text);
    characters += f.text.length;
  }
  return { blocks: doc.fields.length, words, characters };
}

/** Render a field for the active demo into highlighted segments. */
export function segmentsFor(
  demo: DemoId,
  field: MockField,
  find: string,
  replace: string,
): Segment[] {
  switch (demo) {
    case "search-replace":
      return applyReplace(field.text, find, replace);
    case "pseudo":
      return applyPseudo(field.text);
    case "insights":
    default:
      return [{ text: field.text, changed: false }];
  }
}
