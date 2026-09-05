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
//   • Catalog, bilingual and data-config formats → an entry list (key → text),
//     decided by the format's family from the registry, not by a list here.
//   • Generic fallback → the layer/group tree as titled sections of blocks.
//
// Each RenderLine/RenderCell carries the originating block's *source* runs plus
// every *target* locale's runs and the block's overlays/annotations, so the
// renderer can switch source↔target, highlight annotations, and animate
// transitions without re-walking the tree.

import { otherBranch, projectRuns, type RunSpec } from "@neokapi/kapi-format";
import { localeOfVariant } from "../../lib/text-direction";
import { formatFamily } from "./formatFamily";
import type { SpanInfo } from "../../types/span";
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
  /** Source-side inline codes, positioned in `text` (see {@link runsCodes}). */
  codes?: InlineCode[];
  /** Per-locale inline codes, positioned in the matching `targets` entry. */
  targetCodes?: Record<string, InlineCode[]>;
  /** The block's source locale, so the renderer can set `dir`/`lang` on it. */
  sourceLocale?: string;
  /** Render role, used for typography (heading vs body vs bullet vs key vs code). */
  role?: "heading" | "title" | "body" | "bullet" | "key" | "code";
  /**
   * The block's whitespace is significant (code blocks, `<pre>`, preformatted
   * cells): render with `white-space: pre-wrap` rather than collapsing runs of
   * spaces and newlines into prose.
   */
  preserveWhitespace?: boolean;
  /** Code language hint for a `role: "code"` line (e.g. "go", "bash"). */
  codeLanguage?: string;
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
  /**
   * The document's source locale, when the engine stated one. Drives the `dir`
   * / `lang` attributes on the source side, so an RTL *source* document reads
   * correctly too — not only RTL targets.
   */
  sourceLocale?: string;
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

/**
 * A run as the document reads it: a piece of literal text, or a code standing
 * where something that is not text sits.
 *
 * The line's text and the line's inline codes are two views of this one
 * sequence, which is what keeps their offsets aligned — a text projection and a
 * code projection written separately drift the moment one of them learns a new
 * kind of run and the other does not.
 */
type DocumentPiece = { readonly text: string } | { readonly code: SpanInfo };

/**
 * The document reading, declared kind by kind. Nothing may be answered by
 * omission: leave a kind out and this does not compile, and a kind the model
 * gains later breaks every projection until each has said what it does.
 *
 * A plural reads as one form — the same branch `runsPlainText` measures, so a
 * position over this text means what the engine means by it — behind a chip
 * saying so, because a reader must not mistake one form for the whole.
 */
const DOCUMENT_PIECES: RunSpec<Run, DocumentPiece> = {
  text: (r) => ({ text: r.text ?? "" }),
  ph: (r) => ({ code: codeSpan(r) ?? unknownCode("ph") }),
  pcOpen: (r) => ({ code: codeSpan(r) ?? unknownCode("pcOpen") }),
  pcClose: (r) => ({ code: codeSpan(r) ?? unknownCode("pcClose") }),
  sub: (r) => ({ code: codeSpan(r) ?? unknownCode("sub") }),
  plural: {
    expand: (r) => [
      { code: branchCode("plural", r.plural?.pivot) },
      ...projectRuns(otherBranch(r.plural?.forms ?? {}), DOCUMENT_PIECES),
    ],
  },
  select: {
    expand: (r) => [
      { code: branchCode("select", r.select?.pivot) },
      ...projectRuns(otherBranch(r.select?.cases ?? {}), DOCUMENT_PIECES),
    ],
  },
  // A run this build cannot read still occupies its place in the document, as a
  // chip naming what it was. The alternative is the silence this whole
  // declaration exists to prevent.
  fallback: (kind) => ({ code: unknownCode(kind) }),
};

/** A chip standing for a run kind the reading has no better rendering for. */
function unknownCode(kind: string): SpanInfo {
  return { span_type: "placeholder", type: kind, id: kind, data: `⟨${kind}⟩` };
}

/** A chip marking that what follows is one branch of a plural / select. */
function branchCode(kind: "plural" | "select", pivot: string | undefined): SpanInfo {
  const on = pivot ?? "";
  return {
    span_type: "placeholder",
    type: kind,
    id: on || kind,
    data: `{${on}, ${kind}}`,
    equiv_text: on,
  };
}

function documentPieces(runs: Run[] | undefined): DocumentPiece[] {
  return projectRuns(runs, DOCUMENT_PIECES);
}

/** Concatenate a run sequence's literal text (its inline codes ride alongside). */
export function runsText(runs: Run[] | undefined): string {
  let out = "";
  for (const piece of documentPieces(runs)) {
    if ("text" in piece) out += piece.text;
  }
  return out;
}

/**
 * The text AND locale for one side of a node ("source", or a target variant
 * key), paired so a caller can never show one side's text tagged with
 * another's locale. A node with no committed value for `side` reads as its
 * source — and that fallback applies to the locale too, not only the text:
 * a `side` selector and a `sourceLocale` fallback computed by two separate
 * bits of logic are two chances to disagree, which is how a Blocks/Structure/
 * Layout/Subtitle row once rendered Arabic tagged `ltr`. This is the one
 * place that side-selection happens; a caller passes the result straight to
 * {@link DirectionalText}'s `locale` prop, never re-deriving it.
 */
export function blockSideText(
  node: Pick<ContentNode, "source" | "sourceLocale" | "targets">,
  side: string | undefined,
): { text: string; locale: string | undefined } {
  if (side && side !== "source") {
    const t = node.targets?.[side];
    if (t && t.length > 0) return { text: runsText(t), locale: localeOfVariant(side) };
  }
  return { text: runsText(node.source), locale: node.sourceLocale };
}

/**
 * What a rendered line shows for one side, paired the way {@link blockSideText}
 * pairs a node's text and locale: the text, the inline codes positioned in it,
 * and the locale whose writing direction it takes. A line with no target for
 * `side` reads as its source, in the source's locale and with the source's
 * codes; `documentSourceLocale` stands in for a line that carries none of its
 * own. The renderer takes text and direction from this one result, so an
 * untranslated line falling back to its source is never laid out in the
 * target's direction.
 */
export function lineSideText(
  line: Pick<RenderLine, "text" | "codes" | "targets" | "targetCodes" | "sourceLocale">,
  side: string,
  documentSourceLocale?: string,
): { text: string; codes: InlineCode[]; locale: string | undefined; fromTarget: boolean } {
  if (side !== "source") {
    const t = line.targets?.[side];
    if (t)
      return { text: t, codes: line.targetCodes?.[side] ?? [], locale: side, fromTarget: true };
  }
  return {
    text: line.text,
    codes: line.codes ?? [],
    locale: line.sourceLocale ?? documentSourceLocale,
    fromTarget: false,
  };
}

/**
 * One inline code — a placeholder or half a paired code — located in the text
 * `runsText` produces for the same run sequence.
 *
 * Inline codes carry no literal text, so flattening a run sequence leaves them
 * with nowhere to land: a variable, a `<br/>`, a link's opening tag would simply
 * vanish from the reading. Positioning them alongside the text keeps them
 * renderable without putting substitute characters into the string every other
 * consumer (overlay anchoring, word diff, subtitle cues) reads as prose.
 */
export interface InlineCode {
  /** Character offset into the flattened text where the code sits. */
  offset: number;
  /** The code as a span, the shape the vocabulary registry keys chips on. */
  span: SpanInfo;
}

/**
 * The span one inline-code run projects to, or null for a run that carries no
 * code (text, plural, select).
 *
 * Mirrors the run → SpanInfo mapping in `runsCodedBridge.runsToCoded`, over the
 * preview kit's loose local `Run` (see the divergence note in ./types) rather
 * than the strict generated union.
 */
function codeSpan(run: Run): SpanInfo | null {
  const code = run.ph ?? run.pcOpen ?? run.pcClose;
  if (code) {
    return {
      span_type: run.ph ? "placeholder" : run.pcOpen ? "opening" : "closing",
      type: code.type ?? "",
      sub_type: code.subType,
      id: code.id,
      data: code.data ?? "",
      equiv_text: code.equiv,
      display_text: code.disp,
    };
  }
  // A subblock reference is opaque to a reading of this document — its content
  // lives in the block it points at — so it reads as a placeholder for it.
  if (run.sub) {
    const equiv = run.sub.equiv ?? run.sub.ref;
    return {
      span_type: "placeholder",
      type: "sub",
      id: run.sub.id,
      data: `[${equiv}]`,
      equiv_text: equiv,
    };
  }
  return null;
}

/** The inline codes of a run sequence, positioned in its `runsText`. */
export function runsCodes(runs: Run[] | undefined): InlineCode[] {
  const out: InlineCode[] = [];
  let offset = 0;
  for (const piece of documentPieces(runs)) {
    if ("text" in piece) offset += piece.text.length;
    else out.push({ offset, span: piece.code });
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

/** Per-locale inline codes for a block, parallel to {@link targetTexts}. */
function targetCodes(node: ContentNode): Record<string, InlineCode[]> | undefined {
  if (!node.targets) return undefined;
  const out: Record<string, InlineCode[]> = {};
  for (const [loc, runs] of Object.entries(node.targets)) {
    const codes = runsCodes(runs);
    if (codes.length > 0) out[loc] = codes;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/** Build a RenderLine from a block, carrying its targets/overlays/annotations. */
function lineFromBlock(node: ContentNode, role?: RenderLine["role"]): RenderLine {
  const targets = targetTexts(node);
  const codes = runsCodes(node.source);
  const tCodes = targetCodes(node);
  // Code wins over the caller's positional role: a fenced block is code
  // wherever it appears, including as the first paragraph of a document (which
  // `docRole` would otherwise typeset as the title).
  const resolvedRole = isCodeBlock(node) ? "code" : role;
  const codeLanguage = node.properties?.[CODE_LANGUAGE_PROP];
  return {
    id: node.id,
    text: runsText(node.source),
    ...(targets ? { targets } : {}),
    ...(codes.length > 0 ? { codes } : {}),
    ...(tCodes ? { targetCodes: tCodes } : {}),
    ...(node.sourceLocale ? { sourceLocale: node.sourceLocale } : {}),
    ...(resolvedRole ? { role: resolvedRole } : {}),
    // Code is preformatted whether or not the engine set the flag: a code block
    // whose indentation collapses is no longer code.
    ...(node.preserveWhitespace || resolvedRole === "code" ? { preserveWhitespace: true } : {}),
    ...(codeLanguage ? { codeLanguage } : {}),
    ...(node.overlays && node.overlays.length > 0 ? { overlays: node.overlays } : {}),
    ...(node.annotations && node.annotations.length > 0 ? { annotations: node.annotations } : {}),
  };
}

/**
 * The block property carrying a fenced code block's info string
 * (`model.PropCodeLanguage`).
 */
const CODE_LANGUAGE_PROP = "code.language";

/**
 * True when a block is a code block — a fenced or indented markdown block, an
 * HTML `<pre>`, a docx code paragraph. The engine states this twice: as the
 * semantic role on the structure view (`model.RoleCode`) and as the block type
 * the format reader assigned. Either is authoritative; formats that only set
 * one still render as code.
 */
function isCodeBlock(node: ContentNode): boolean {
  if (node.structure?.role?.toLowerCase() === "code") return true;
  const t = (node.type ?? "").toLowerCase();
  return t === "code" || t === "code-block" || t === "codeblock" || t === "pre";
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

/** The first source locale the engine stated on any block, if any. */
function collectSourceLocale(tree: ContentTree): string | undefined {
  let found: string | undefined;
  eachNode(tree.root, (n) => {
    if (found !== undefined || n.kind !== "block") return;
    if (n.sourceLocale) found = n.sourceLocale;
  });
  return found;
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
const SHARED_STRINGS_RE = /\/sharedStrings\.xml$/i;
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

/**
 * Index the translatable shared-string blocks (under sharedStrings.xml) by their
 * siIndex, so a worksheet cell anchor can be joined back to the block that
 * actually holds the source runs, every target locale, and the stand-off
 * overlays. A single shared string backs many cells, so this is the bridge that
 * lets the grid show translations without duplicating the text per cell.
 */
function sharedStringsByIndex(tree: ContentTree): Map<string, ContentNode> {
  const byLayer = blocksForLayer(tree, (n) => SHARED_STRINGS_RE.test(n));
  const map = new Map<string, ContentNode>();
  for (const blocks of byLayer.values()) {
    for (const b of blocks) {
      const si = b.properties?.siIndex;
      if (si !== undefined) map.set(si, b);
    }
  }
  return map;
}

function extractSheets(tree: ContentTree): RenderSheet[] {
  const byLayer = blocksForLayer(tree, (n) => WORKSHEET_RE.test(n));
  const shared = sharedStringsByIndex(tree);
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
      // A shared-string cell anchor carries only the resolved source text; the
      // translatable block (with targets, overlays and rich-text runs) lives in
      // sharedStrings.xml. Render from that block when the cell references one,
      // so the grid shows translations and inline formatting — but keep the
      // cell's own id and position (one block backs many cells).
      const si = b.properties?.siIndex;
      const source = si !== undefined ? (shared.get(si) ?? b) : b;
      cells.push({
        ...cellLine(b, source),
        id: b.id,
        col,
        row,
        ref: ref || colLabel(col) + (row + 1),
      });
      maxCol = Math.max(maxCol, col);
      maxRow = Math.max(maxRow, row);
    }
    return { name, cols: maxCol + 1, rows: maxRow + 1, cells };
  });
}

/**
 * The block property carrying a value cell's formatted display
 * (`model.PropCellDisplay`): what the sheet shows once the cell's number
 * format is applied to the stored value. A date is stored as a serial day
 * count, a percentage as a fraction; the stored value stays in the runs and
 * the display travels beside it.
 */
const CELL_DISPLAY_PROP = "cell.display";

/**
 * A cell's render line. A text cell renders its (shared-string or inline)
 * block with its targets and codes; a value cell renders its formatted
 * display, a single string the sheet computed, which carries no inline codes
 * and has no locale variant.
 */
function cellLine(cell: ContentNode, source: ContentNode): RenderLine {
  const line = lineFromBlock(source);
  const display = cell.properties?.[CELL_DISPLAY_PROP];
  if (display === undefined) return line;
  return { ...line, text: display, codes: [] };
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

/**
 * Whether a format's blocks read as a flowing document page or as an entry
 * list, asked of the format registry rather than of a list kept here.
 *
 * The registry states the content shape of every format it knows
 * (registry.FormatFamily, generated into @neokapi/reference-data), so a format
 * added to the engine gets the right reading without a second declaration in
 * the frontend. Marked-up text and word-processor documents flow; catalogs,
 * bilingual stores and the structured-data carriers are entries.
 */
function readsAsDocument(format: string): boolean {
  const family = formatFamily(format);
  return family === "rich-markup" || family === "office-doc";
}

function readsAsEntryList(format: string): boolean {
  const family = formatFamily(format);
  return (
    family === "catalog-keyvalue" || family === "bilingual-interchange" || family === "data-config"
  );
}

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
  const sourceLocale = collectSourceLocale(tree);
  const ctx = { locales, format };
  // Every branch below states the same source locale, so stamp it once here
  // rather than threading it through the per-kind extractors.
  const withSource = (doc: RenderDoc): RenderDoc => (sourceLocale ? { ...doc, sourceLocale } : doc);

  for (const rule of rules) {
    const doc = rule.detect(tree, ctx);
    if (doc) return withSource(doc);
  }

  // No structured shape matched — fall back by format family.
  const blocks = allBlocks(tree);

  if (readsAsDocument(format)) {
    const paragraphs = blocks.map((b, i) => lineFromBlock(b, docRole(b, i)));
    return withSource({ kind: "doc", format, locales, paragraphs });
  }

  if (readsAsEntryList(format)) {
    const lines = blocks.map((b) => {
      const l = lineFromBlock(b, "key");
      const k = entryKey(b);
      return k ? { ...l, key: k } : l;
    });
    return withSource({ kind: "list", format, locales, lines });
  }

  // Truly generic: render the layer/group hierarchy as titled sections. If the
  // tree is flat (no containers), fall back to a plain entry list.
  const sections = extractSections(tree);
  if (sections.length > 0) {
    return withSource({ kind: "sections", format, locales, sections });
  }
  const lines = blocks.map((b) => {
    const l = lineFromBlock(b, "key");
    const k = entryKey(b);
    return k ? { ...l, key: k } : l;
  });
  return withSource({ kind: "list", format, locales, lines });
}
