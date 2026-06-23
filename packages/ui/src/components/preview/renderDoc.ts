// treeToRenderDoc — normalize a ContentTree (the wasm `labInspect` /
// `labInspectAnnotated` / `runtime.inspect` output) into a *structured document
// render model* a human would recognize: a deck of slides, a spreadsheet grid, a
// page of paragraphs, paged documents, or an entry list. This is the bridge
// between the engine's faithful content model and the FormatPreview renderer.
//
// Everything here is derived from the REAL extraction structure (verified
// against the engine), so the renderer shows what kapi actually pulled out of
// the file — not a hand-drawn mock. The dispatch is data-driven (a table of
// structure detectors, see STRUCTURE_RULES) so a new format that produces a
// recognizable layer shape can be added in one place, and any format that the
// table doesn't recognize degrades gracefully to a sectioned block render.
//
//   • PPTX → openxml layer "ppt/slides/slideN.xml"; blocks are paragraphs in
//     reading order (first = title, rest = body bullets). slideLayouts/*,
//     slideMasters/* and docProps/* boilerplate layers are ignored.
//   • XLSX → openxml layer "xl/worksheets/sheetN.xml"; each cell is a block with
//     properties.cell ("A1"). We parse the ref into (col,row) and place the text
//     in a real grid, filling blank gaps. docProps/* are ignored.
//   • DOCX → layer "word/document.xml"; blocks are paragraphs in order.
//   • PDF / paged formats → one page per page layer (properties.page or a
//     "page N" layer), else a single doc page.
//   • Markdown / HTML → a rendered document page (heading/bullet roles honored).
//   • JSON / PO / properties / XLIFF / resx / … → an entry list (key → text).
//   • Generic fallback → the layer/group tree as titled sections of blocks.
//
// Each RenderLine/RenderCell carries the originating block's *source* runs plus
// every *target* locale's runs and the block's overlays/annotations, so the
// renderer can switch source↔target, highlight annotations, and animate
// transitions without re-walking the tree.

import type { AnnotationView, ContentNode, ContentTree, OverlayView, Run } from "./types";

// ── Render model ─────────────────────────────────────────────────────────────

export type RenderKind = "slides" | "sheet" | "doc" | "pages" | "list" | "sections";

/**
 * One renderable unit (paragraph, slide line, cell, list entry), tagged with its
 * source block so before/after diffs and variant switching align by id. It
 * carries the source text plus every target locale's text, and the block's
 * stand-off overlays + annotations, so the renderer is purely presentational.
 */
export interface RenderLine {
  /** The id of the originating block, so before/after diffs align by id. */
  id: string;
  /** Source-side text (concatenated literal runs). */
  text: string;
  /** Per-locale target text, keyed by variant locale (e.g. "fr-FR"). */
  targets?: Record<string, string>;
  /** Render role, used for typography (heading vs body vs bullet vs key). */
  role?: "heading" | "title" | "body" | "bullet" | "key";
  /** Optional key/label (JSON path, PO msgid, properties key) shown beside text. */
  key?: string;
  /** Stand-off overlays anchored to this block's runs (terms, entities, qa, …). */
  overlays?: OverlayView[];
  /** Block-level annotations (notes, alt-translations, …). */
  annotations?: AnnotationView[];
}

export interface RenderSlide {
  /** The slide part path (e.g. "ppt/slides/slide1.xml"). */
  name: string;
  title?: RenderLine;
  bullets: RenderLine[];
}

/** One spreadsheet cell placed at a (col,row); blank cells are omitted. */
export interface RenderCell extends RenderLine {
  /** Zero-based column index (A=0, B=1, …). */
  col: number;
  /** Zero-based row index (row "1" → 0). */
  row: number;
  ref: string;
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

/** A single page of a paged document (pdf, multi-page docx, …). */
export interface RenderPage {
  /** A human label for the page (e.g. "Page 1"). */
  name: string;
  lines: RenderLine[];
}

/** A titled section of blocks (the generic structural fallback). */
export interface RenderSection {
  /** The layer/group name that titles this section. */
  name: string;
  /** Nesting depth (0 = top-level), used to indent the section heading. */
  depth: number;
  lines: RenderLine[];
}

export interface RenderDoc {
  kind: RenderKind;
  /** The detected engine format (e.g. "openxml", "markdown"). */
  format: string;
  /** All target locales found across the document, in first-seen order. */
  locales?: string[];
  /** Populated when kind === "slides". */
  slides?: RenderSlide[];
  /** Populated when kind === "sheet" (first/primary worksheet). */
  sheet?: RenderSheet;
  /** All worksheets, when kind === "sheet" and the workbook has several. */
  sheets?: RenderSheet[];
  /** Populated when kind === "doc". */
  paragraphs?: RenderLine[];
  /** Populated when kind === "pages". */
  pages?: RenderPage[];
  /** Populated when kind === "list". */
  lines?: RenderLine[];
  /** Populated when kind === "sections" (the generic fallback). */
  sections?: RenderSection[];
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

/** Per-locale target text for a block (locale → concatenated literal text). */
function targetTexts(node: ContentNode): Record<string, string> | undefined {
  if (!node.targets) return undefined;
  const out: Record<string, string> = {};
  for (const [loc, runs] of Object.entries(node.targets)) {
    out[loc] = runsText(runs);
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/** Build a RenderLine from a block, carrying its targets/overlays/annotations. */
function lineFromBlock(node: ContentNode, role?: RenderLine["role"]): RenderLine {
  const targets = targetTexts(node);
  return {
    id: node.id,
    text: runsText(node.source),
    ...(targets ? { targets } : {}),
    ...(role ? { role } : {}),
    ...(node.overlays && node.overlays.length > 0 ? { overlays: node.overlays } : {}),
    ...(node.annotations && node.annotations.length > 0 ? { annotations: node.annotations } : {}),
  };
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

/** All translatable blocks in document order, regardless of layer. */
function allBlocks(tree: ContentTree): ContentNode[] {
  const out: ContentNode[] = [];
  eachNode(tree.root, (n) => {
    if (n.kind === "block") out.push(n);
  });
  return out;
}

/** Every target locale across the document, in first-seen order. */
function collectLocales(tree: ContentTree): string[] {
  const seen = new Set<string>();
  const order: string[] = [];
  eachNode(tree.root, (n) => {
    if (n.kind !== "block" || !n.targets) return;
    for (const loc of Object.keys(n.targets)) {
      if (!seen.has(loc)) {
        seen.add(loc);
        order.push(loc);
      }
    }
  });
  return order;
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

/** Natural sort by trailing number so "slide2" precedes "slide10". */
function byTrailingNumber(a: string, b: string): number {
  const na = parseInt(/(\d+)\.xml$/i.exec(a)?.[1] ?? /(\d+)\D*$/.exec(a)?.[1] ?? "0", 10);
  const nb = parseInt(/(\d+)\.xml$/i.exec(b)?.[1] ?? /(\d+)\D*$/.exec(b)?.[1] ?? "0", 10);
  return na - nb;
}

/** Map a block's type to a render role (markdown/docx/html fallback). */
function docRole(b: ContentNode, index: number): RenderLine["role"] {
  const t = (b.type ?? "").toLowerCase();
  if (t === "heading" || /^h[1-6]$/.test(t)) return "heading";
  if (t.includes("list") || t.includes("bullet")) return "bullet";
  if (t === "paragraph" && index === 0) return "heading";
  return "body";
}

/** A short entry key for the list view (the most descriptive property).
 *
 * Structured/catalog formats anchor each value to a key: JSON/YAML/properties
 * carry the dotted key path on the block's `name` (and JSON also on
 * `json.keypath`), gettext on `msgid`, etc. Surfacing it turns the flat list
 * into a key → value view. Prose formats leave `name` empty, so they stay plain
 * text. */
export function entryKey(b: ContentNode): string | undefined {
  const p = b.properties ?? {};
  return p.key ?? p.path ?? p.name ?? p.id ?? p.msgid ?? p["json.keypath"] ?? b.name ?? undefined;
}

// ── Per-kind extraction ──────────────────────────────────────────────────────

const SLIDE_RE = /^ppt\/slides\/slide\d+\.xml$/i;
const WORKSHEET_RE = /^xl\/worksheets\/sheet\d+\.xml$/i;
const DOCX_BODY_RE = /^word\/document\.xml$/i;

function extractSlides(tree: ContentTree): RenderSlide[] {
  const byLayer = blocksForLayer(tree, (n) => SLIDE_RE.test(n));
  const names = [...byLayer.keys()].sort(byTrailingNumber);
  return names.map((name) => {
    const blocks = byLayer.get(name) ?? [];
    const lines = blocks.map((b, i) => lineFromBlock(b, i === 0 ? "title" : "bullet"));
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
      cells.push({ ...lineFromBlock(b), col, row, ref: ref || colLabel(col) + (row + 1) });
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
  return blocks.map((b, i) => lineFromBlock(b, docRole(b, i)));
}

const PAGE_LAYER_RE = /\bpage\s*\d+\b/i;

/** Group blocks into pages by properties.page or a "page N" layer name. */
function extractPages(tree: ContentTree): RenderPage[] {
  // Prefer explicit per-block page numbers (pdf), else page layers.
  const byPageProp = new Map<string, ContentNode[]>();
  let sawPageProp = false;
  eachNode(tree.root, (n) => {
    if (n.kind !== "block") return;
    const pg = n.properties?.page;
    if (pg !== undefined) {
      sawPageProp = true;
      const bucket = byPageProp.get(pg) ?? [];
      bucket.push(n);
      byPageProp.set(pg, bucket);
    }
  });
  if (sawPageProp) {
    const keys = [...byPageProp.keys()].sort((a, b) => Number(a) - Number(b));
    return keys.map((k) => ({
      name: `Page ${k}`,
      lines: (byPageProp.get(k) ?? []).map((b, i) => lineFromBlock(b, docRole(b, i))),
    }));
  }
  const byLayer = blocksForLayer(tree, (n) => PAGE_LAYER_RE.test(n));
  if (byLayer.size === 0) return [];
  const names = [...byLayer.keys()].sort(byTrailingNumber);
  return names.map((name) => ({
    name: PAGE_LAYER_RE.exec(name)?.[0] ?? name,
    lines: (byLayer.get(name) ?? []).map((b, i) => lineFromBlock(b, docRole(b, i))),
  }));
}

/** The generic structural fallback: titled sections per layer/group. */
function extractSections(tree: ContentTree): RenderSection[] {
  const sections: RenderSection[] = [];
  const walk = (nodes: ContentNode[] | undefined, depth: number, owner?: RenderSection) => {
    if (!nodes) return;
    for (const n of nodes) {
      if (n.kind === "layer" || n.kind === "group") {
        const sec: RenderSection = { name: n.name ?? n.id, depth, lines: [] };
        sections.push(sec);
        walk(n.children, depth + 1, sec);
      } else if (n.kind === "block") {
        owner?.lines.push(lineFromBlock(n, docRole(n, owner.lines.length)));
      }
    }
  };
  walk(tree.root, 0);
  return sections.filter((s) => s.lines.length > 0);
}

// ── Structure dispatch table (data-driven + extensible) ──────────────────────

/**
 * A structure rule recognizes a document shape from the engine's layer tree and
 * produces a RenderDoc. Rules are tried in order; the first that returns a
 * non-null doc wins. Add a new format's shape here — everything else degrades
 * gracefully to the section/list fallback below.
 */
export interface StructureRule {
  /** A stable id (for testing / extension overrides). */
  id: string;
  detect: (tree: ContentTree, ctx: { locales: string[]; format: string }) => RenderDoc | null;
}

export const STRUCTURE_RULES: StructureRule[] = [
  {
    id: "slides",
    detect: (tree, { locales, format }) => {
      const slides = extractSlides(tree);
      return slides.length > 0 ? { kind: "slides", format, locales, slides } : null;
    },
  },
  {
    id: "sheet",
    detect: (tree, { locales, format }) => {
      const sheets = extractSheets(tree);
      return sheets.length > 0
        ? { kind: "sheet", format, locales, sheet: sheets[0], sheets }
        : null;
    },
  },
  {
    id: "doc",
    detect: (tree, { locales, format }) => {
      const paragraphs = extractDocParagraphs(tree);
      return paragraphs.length > 0 ? { kind: "doc", format, locales, paragraphs } : null;
    },
  },
  {
    id: "pages",
    detect: (tree, { locales, format }) => {
      const pages = extractPages(tree);
      return pages.length > 0 ? { kind: "pages", format, locales, pages } : null;
    },
  },
];

/** Formats whose blocks read best as a flowing document page rather than a list. */
const DOC_FORMATS = new Set(["markdown", "md", "mdx", "html", "htm"]);
/** Formats that are key→value catalogs / bilingual stores → an entry list. */
const LIST_FORMATS = new Set([
  "json",
  "yaml",
  "properties",
  "po",
  "xliff",
  "xliff2",
  "resx",
  "arb",
  "xcstrings",
  "i18next",
  "androidxml",
  "applestrings",
  "designtokens",
  "csv",
]);

// ── Public entry point ───────────────────────────────────────────────────────

/**
 * Normalize a ContentTree into a structured document render model. Dispatches on
 * the format + the layer shape the engine produced (the STRUCTURE_RULES table),
 * never on the file name. Unknown shapes degrade to a flowing doc (markup), an
 * entry list (catalogs / bilingual), or a sectioned block render (generic).
 */
export function treeToRenderDoc(
  tree: ContentTree,
  rules: StructureRule[] = STRUCTURE_RULES,
): RenderDoc {
  const format = tree.format ?? "";
  const locales = collectLocales(tree);
  const ctx = { locales, format };

  for (const rule of rules) {
    const doc = rule.detect(tree, ctx);
    if (doc) return doc;
  }

  // No structured shape matched — fall back by format family.
  const blocks = allBlocks(tree);

  if (DOC_FORMATS.has(format)) {
    const paragraphs = blocks.map((b, i) => lineFromBlock(b, docRole(b, i)));
    return { kind: "doc", format, locales, paragraphs };
  }

  if (LIST_FORMATS.has(format)) {
    const lines = blocks.map((b) => {
      const l = lineFromBlock(b, "key");
      const k = entryKey(b);
      return k ? { ...l, key: k } : l;
    });
    return { kind: "list", format, locales, lines };
  }

  // Truly generic: render the layer/group hierarchy as titled sections. If the
  // tree is flat (no containers), fall back to a plain entry list.
  const sections = extractSections(tree);
  if (sections.length > 0) {
    return { kind: "sections", format, locales, sections };
  }
  const lines = blocks.map((b) => {
    const l = lineFromBlock(b, "key");
    const k = entryKey(b);
    return k ? { ...l, key: k } : l;
  });
  return { kind: "list", format, locales, lines };
}
