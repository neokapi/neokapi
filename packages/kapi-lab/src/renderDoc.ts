// treeToRenderDoc — normalize a ContentTree (the wasm `labInspect` /
// `runtime.inspect` output) into a *structured document render model* a human
// would recognize: a deck of slides, a spreadsheet grid, or a page of
// paragraphs. This is the bridge between the engine's faithful content model and
// the "before / after" document renderers used on the docs landing.
//
// Everything here is derived from the REAL extraction structure (verified
// against the engine), so the renderer shows what kapi actually pulled out of
// the file — not a hand-drawn mock:
//
//   • XLSX → openxml layer "xl/worksheets/sheetN.xml"; each cell is a block with
//     type:"cell" and properties.cell ("A1"). We parse the ref into (col,row)
//     and place the text in a real grid, filling blank gaps. docProps/* and
//     other non-worksheet layers are ignored.
//   • PPTX → openxml layer "ppt/slides/slideN.xml"; blocks are type:"paragraph"
//     in reading order (first = title, rest = body bullets). slideLayouts/*,
//     slideMasters/* and docProps/* layers carry placeholder boilerplate and are
//     ignored — only ppt/slides/slideN.xml render.
//   • DOCX → layer "word/document.xml"; blocks are paragraphs in order. The first
//     is treated as a heading when it looks like one (a short, single line).
//   • Markdown / JSON / anything else → fallback: render the blocks as a simple
//     document (headings/list-items honored) or a key list.

import type { ContentNode, ContentTree, Run } from "./types";

// ── Render model ─────────────────────────────────────────────────────────────

export type RenderKind = "slides" | "sheet" | "doc" | "list";

/** One paragraph/line in a slide, doc, or list, tagged with its source block. */
export interface RenderLine {
  /** The id of the originating block, so before/after diffs align by id. */
  id: string;
  text: string;
  /** Render role, used for typography (heading vs body vs bullet vs key). */
  role?: "heading" | "title" | "body" | "bullet" | "key";
}

export interface RenderSlide {
  /** The slide part path (e.g. "ppt/slides/slide1.xml"). */
  name: string;
  title?: RenderLine;
  bullets: RenderLine[];
}

/** One spreadsheet cell placed at a (col,row); blank cells are omitted. */
export interface RenderCell {
  id: string;
  /** Zero-based column index (A=0, B=1, …). */
  col: number;
  /** Zero-based row index (row "1" → 0). */
  row: number;
  ref: string;
  text: string;
}

export interface RenderSheet {
  /** The worksheet name (the layer's part path, e.g. "xl/worksheets/sheet1.xml"). */
  name: string;
  /** Number of columns the populated cells span. */
  cols: number;
  /** Number of rows the populated cells span. */
  rows: number;
  cells: RenderCell[];
}

export interface RenderDoc {
  kind: RenderKind;
  /** The detected engine format (e.g. "openxml", "markdown"). */
  format: string;
  /** Populated when kind === "slides". */
  slides?: RenderSlide[];
  /** Populated when kind === "sheet" (first/primary worksheet). */
  sheet?: RenderSheet;
  /** All worksheets, when kind === "sheet" and the workbook has several. */
  sheets?: RenderSheet[];
  /** Populated when kind === "doc". */
  paragraphs?: RenderLine[];
  /** Populated when kind === "list" (the JSON/other fallback). */
  lines?: RenderLine[];
}

// ── Helpers ──────────────────────────────────────────────────────────────────

/** Concatenate a run sequence's literal text (ignoring inline markup runs). */
export function runsText(runs: Run[] | undefined): string {
  if (!runs) return "";
  let out = "";
  for (const r of runs) {
    if (typeof r.text === "string") out += r.text;
  }
  return out;
}

/** Walk the tree depth-first, invoking `visit(node, layerName)` for each node. */
function eachNode(
  nodes: ContentNode[] | undefined,
  visit: (node: ContentNode, layerName: string | undefined) => void,
  layerName?: string,
): void {
  if (!nodes) return;
  for (const n of nodes) {
    const childLayer = n.kind === "layer" ? n.name : layerName;
    visit(n, layerName);
    eachNode(n.children, visit, childLayer);
  }
}

/** Collect the blocks directly belonging to a layer whose name matches `pred`. */
function blocksForLayer(
  tree: ContentTree,
  pred: (name: string) => boolean,
): Map<string, ContentNode[]> {
  const byLayer = new Map<string, ContentNode[]>();
  eachNode(tree.root, (node, layerName) => {
    if (node.kind !== "block") return;
    if (!layerName || !pred(layerName)) return;
    const bucket = byLayer.get(layerName) ?? [];
    bucket.push(node);
    byLayer.set(layerName, bucket);
  });
  return byLayer;
}

/** Parse a spreadsheet cell ref ("A1", "AB12") into zero-based (col,row). */
export function parseCellRef(ref: string): { col: number; row: number } | null {
  const m = /^([A-Za-z]+)(\d+)$/.exec(ref.trim());
  if (!m) return null;
  const letters = m[1].toUpperCase();
  let col = 0;
  for (const ch of letters) {
    col = col * 26 + (ch.charCodeAt(0) - 64); // 'A' → 1
  }
  const row = parseInt(m[2], 10);
  if (col < 1 || row < 1) return null;
  return { col: col - 1, row: row - 1 };
}

/** Convert a zero-based column index back to spreadsheet letters (0 → "A"). */
export function colLabel(col: number): string {
  let n = col + 1;
  let out = "";
  while (n > 0) {
    const rem = (n - 1) % 26;
    out = String.fromCharCode(65 + rem) + out;
    n = Math.floor((n - 1) / 26);
  }
  return out;
}

// ── Per-kind extraction ──────────────────────────────────────────────────────

const SLIDE_RE = /^ppt\/slides\/slide\d+\.xml$/i;
const WORKSHEET_RE = /^xl\/worksheets\/sheet\d+\.xml$/i;
const DOCX_BODY_RE = /^word\/document\.xml$/i;

/** Natural sort by trailing number so "slide2" precedes "slide10". */
function byTrailingNumber(a: string, b: string): number {
  const na = parseInt(/(\d+)\.xml$/i.exec(a)?.[1] ?? "0", 10);
  const nb = parseInt(/(\d+)\.xml$/i.exec(b)?.[1] ?? "0", 10);
  return na - nb;
}

function extractSlides(tree: ContentTree): RenderSlide[] {
  const byLayer = blocksForLayer(tree, (n) => SLIDE_RE.test(n));
  const names = [...byLayer.keys()].sort(byTrailingNumber);
  return names.map((name) => {
    const blocks = byLayer.get(name) ?? [];
    const lines: RenderLine[] = blocks.map((b, i) => ({
      id: b.id,
      text: runsText(b.source),
      role: i === 0 ? "title" : "bullet",
    }));
    const [title, ...bullets] = lines;
    return { name, title, bullets };
  });
}

function extractSheets(tree: ContentTree): RenderSheet[] {
  const byLayer = blocksForLayer(tree, (n) => WORKSHEET_RE.test(n));
  const names = [...byLayer.keys()].sort(byTrailingNumber);
  return names.map((name) => {
    const blocks = byLayer.get(name) ?? [];
    const cells: RenderCell[] = [];
    let maxCol = 0;
    let maxRow = 0;
    let fallbackRow = 0;
    for (const b of blocks) {
      const ref = b.properties?.cell ?? "";
      const pos = ref ? parseCellRef(ref) : null;
      const col = pos ? pos.col : 0;
      const row = pos ? pos.row : fallbackRow++;
      cells.push({
        id: b.id,
        col,
        row,
        ref: ref || colLabel(col) + (row + 1),
        text: runsText(b.source),
      });
      maxCol = Math.max(maxCol, col);
      maxRow = Math.max(maxRow, row);
    }
    return { name, cols: maxCol + 1, rows: maxRow + 1, cells };
  });
}

function extractDocParagraphs(tree: ContentTree): RenderLine[] {
  const byLayer = blocksForLayer(tree, (n) => DOCX_BODY_RE.test(n));
  const name = [...byLayer.keys()][0];
  const blocks = name ? (byLayer.get(name) ?? []) : [];
  return blocks.map((b, i) => ({
    id: b.id,
    text: runsText(b.source),
    role: docRole(b, i),
  }));
}

/** All translatable blocks in document order, regardless of layer. */
function allBlocks(tree: ContentTree): ContentNode[] {
  const out: ContentNode[] = [];
  eachNode(tree.root, (n) => {
    if (n.kind === "block") out.push(n);
  });
  return out;
}

/** Map a block's type to a render role (markdown/docx fallback). */
function docRole(b: ContentNode, index: number): RenderLine["role"] {
  const t = (b.type ?? "").toLowerCase();
  if (t === "heading") return "heading";
  if (t.includes("list")) return "bullet";
  if (t === "paragraph" && index === 0) return "heading";
  return "body";
}

// ── Public entry point ───────────────────────────────────────────────────────

/**
 * Normalize a ContentTree into a structured document render model. Dispatches on
 * the format + the layer shape the engine produced — never on the file name.
 */
export function treeToRenderDoc(tree: ContentTree): RenderDoc {
  const format = tree.format ?? "";

  // PowerPoint: real slide layers present → a deck.
  const slides = extractSlides(tree);
  if (slides.length > 0) {
    return { kind: "slides", format, slides };
  }

  // Excel: real worksheet layers present → a sheet.
  const sheets = extractSheets(tree);
  if (sheets.length > 0) {
    return { kind: "sheet", format, sheet: sheets[0], sheets };
  }

  // Word: a document body layer → a page of paragraphs.
  const docParas = extractDocParagraphs(tree);
  if (docParas.length > 0) {
    return { kind: "doc", format, paragraphs: docParas };
  }

  // Fallback: render every block. Markdown gets heading/bullet roles; JSON and
  // other catalog formats become a key list (one block per line).
  const blocks = allBlocks(tree);
  const lines: RenderLine[] = blocks.map((b, i) => ({
    id: b.id,
    text: runsText(b.source),
    role: docRole(b, i),
  }));
  if (format === "markdown") {
    return { kind: "doc", format, paragraphs: lines };
  }
  return { kind: "list", format, lines };
}
