import React, { useMemo } from "react";
import { cn } from "../../lib/utils";
import { directionAttrs, type TextDirection } from "../../lib/text-direction";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "../ui/tooltip";
import { semanticLabel, semanticTooltip, tagColors } from "../editor/tagSemantics";
import type {
  InlineCode,
  RenderCell,
  RenderDoc,
  RenderLine,
  RenderPage,
  RenderSection,
  RenderSheet,
  RenderSlide,
} from "./renderDoc";
import { colLabel, treeToRenderDoc } from "./renderDoc";
import RenderedDocument from "./RenderedDocument";
import type { ContentTree } from "./types";
import {
  resolveOverlaySpans,
  segmentText,
  type ResolvedSpan,
  type TextSegment,
} from "./overlayHighlight";
import {
  useTextTransition,
  type TransitionEffect,
  type TypewriterGranularity,
} from "./useTextTransition";
import { SlotText } from "slot-text/react";
import type { SlotOptions } from "slot-text";
import "slot-text/style.css";
import styles from "./FormatPreview.module.css";

// FormatPreview — a structure-aware, annotation-aware preview that renders ANY
// neokapi format from a ContentTree (or a normalized RenderDoc). The structural
// shape (slides / sheet / pages / doc / list / sections) is decided by
// treeToRenderDoc's data-driven dispatch, so a new format degrades gracefully.
//
// It renders either the source runs or a chosen target locale's runs, highlights
// each block's stand-off overlays (color-coded by type, with a tooltip), and
// animates the source→target swap with a crossfade, typewriter, or slot-text
// roll effect (honoring prefers-reduced-motion).

/** Which side of each block to render. */
export type PreviewSide = "source" | (string & {});

/**
 * Host-supplied decoration for one block's element: a class name plus any
 * `data-*` markers. The kit knows nothing about what a host encodes there —
 * review status, a batch mark, a test hook — it only puts the attributes on the
 * element that carries the block, beside the `data-block-id` it always emits.
 */
export type BlockAttrs = { className?: string } & {
  [attr: `data-${string}`]: string | undefined;
};

export interface FormatPreviewProps {
  /** The engine output to render. Provide one of `tree` or `doc`. */
  tree?: ContentTree;
  /** A pre-normalized render model (overrides `tree` when both are given). */
  doc?: RenderDoc;
  /** The "before" model; when present, changed words are highlighted vs it. */
  before?: RenderDoc;
  /** Which side to render — "source" or a target locale key (default "source"). */
  side?: PreviewSide;
  /** Show overlay highlights (default true). */
  annotations?: boolean;
  /**
   * Render the document flush to its container — drop the inner slide/page frame
   * (border, radius, inter-slide gap) so the content bleeds edge to edge. Used
   * for thumbnails where the host (e.g. a grid card) supplies the only frame.
   */
  flush?: boolean;
  /** Restrict overlay highlights to these types (undefined = all). */
  overlayTypes?: string[];
  /** Source→target transition effect (default "none"). */
  transition?: TransitionEffect;
  /** Typewriter granularity when transition === "typewriter" (default "word"). */
  typewriter?: TypewriterGranularity;
  /**
   * Stagger each line's typewriter/slot start by `line index * this` ms, so lines
   * reveal one after another instead of all at once (default 0 = simultaneous).
   */
  typewriterStagger?: number;
  /** Force reduced-motion (instant) — for tests. */
  reducedMotion?: boolean;
  /** Show spreadsheet column letters / row numbers (default true). */
  gridHeaders?: boolean;
  /**
   * Called with a block's id when its element is activated (click, Enter or
   * Space). Present, every block becomes a button-roled, focusable target — the
   * document itself is the way to select a block, so a host needs no list
   * beside it.
   */
  onSelectBlock?: (id: string) => void;
  /** The currently selected block, marked `aria-current` and styled as such. */
  selectedBlockId?: string;
  /** Per-block class name and `data-*` markers (see BlockAttrs). */
  blockAttrs?: (id: string) => BlockAttrs | undefined;
  className?: string;
}

// ── Context for the leaf <LineText> renderer ─────────────────────────────────

interface PreviewCtx {
  side: PreviewSide;
  annotations: boolean;
  overlayFilter?: Set<string>;
  transition: TransitionEffect;
  typewriter: TypewriterGranularity;
  stagger: number;
  reducedMotion?: boolean;
  beforeIndex: Map<string, string> | null;
  /** The document's source locale, used for `dir`/`lang` on the source side. */
  sourceLocale?: string;
  onSelectBlock?: (id: string) => void;
  selectedBlockId?: string;
  blockAttrs?: (id: string) => BlockAttrs | undefined;
}

const Ctx = React.createContext<PreviewCtx | null>(null);

function useCtx(): PreviewCtx {
  const c = React.useContext(Ctx);
  if (!c) throw new Error("FormatPreview line rendered outside provider");
  return c;
}

// ── Writing direction ────────────────────────────────────────────────────────

/**
 * The locale of the text currently rendered for a line: the chosen target
 * locale, or the block's (else the document's) source locale on the source side.
 */
function activeLocale(line: RenderLine | undefined, ctx: PreviewCtx): string | undefined {
  if (ctx.side !== "source") return ctx.side;
  return line?.sourceLocale ?? ctx.sourceLocale;
}

/**
 * `dir` / `lang` for a line's block element. Code is always laid out
 * left-to-right — source code has no RTL reading order even inside an RTL
 * document, and mirroring it would scramble brackets and indentation.
 */
function lineDirAttrs(line: RenderLine, ctx: PreviewCtx): { dir: TextDirection; lang?: string } {
  if (line.role === "code") return { dir: "ltr" };
  return directionAttrs(activeLocale(line, ctx));
}

// ── Block element: identity, host decoration, selection ─────────────────────

/** The attributes every block element carries, whatever structure renders it. */
type BlockElProps = Record<string, unknown> & { className?: string };

/**
 * `useBlockProps` gives one block's element its identity (`data-block-id`), the
 * host's decoration, and — when the host listens for selection — button
 * semantics: focusable, activated by click, Enter or Space, and marked
 * `aria-current` while selected.
 *
 * A click that ends a text selection is not a selection of the block: the
 * reader was highlighting a phrase (to mark a term, say), and stealing that
 * gesture would close the reading over it.
 */
function useBlockProps(id: string, ownClass?: string): BlockElProps {
  const ctx = useCtx();
  const { onSelectBlock } = ctx;
  const decoration = ctx.blockAttrs?.(id);
  const { className: hostClass, ...dataAttrs } = decoration ?? {};

  const activate = React.useCallback(() => {
    if (!onSelectBlock) return;
    if (typeof window !== "undefined" && window.getSelection()?.isCollapsed === false) return;
    onSelectBlock(id);
  }, [onSelectBlock, id]);

  const onKeyDown = React.useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      e.preventDefault();
      activate();
    },
    [activate],
  );

  const selected = ctx.selectedBlockId === id;
  const base: BlockElProps = {
    ...dataAttrs,
    "data-block-id": id,
    className: cn(
      ownClass,
      onSelectBlock && styles.selectable,
      selected && styles.selected,
      hostClass,
    ),
  };
  if (!onSelectBlock) return base;
  return {
    ...base,
    role: "button",
    tabIndex: 0,
    "aria-current": selected ? "true" : undefined,
    onClick: activate,
    onKeyDown,
  };
}

// ── before-model lookup (word diff) ──────────────────────────────────────────

function indexById(doc: RenderDoc | undefined): Map<string, string> | null {
  if (!doc) return null;
  const map = new Map<string, string>();
  const add = (l: RenderLine | undefined) => {
    if (l) map.set(l.id, l.text);
  };
  doc.slides?.forEach((s) => {
    add(s.title);
    s.bullets.forEach(add);
  });
  (doc.sheets ?? (doc.sheet ? [doc.sheet] : [])).forEach((sh) => sh.cells.forEach(add));
  doc.paragraphs?.forEach(add);
  doc.pages?.forEach((p) => p.lines.forEach(add));
  doc.sections?.forEach((s) => s.lines.forEach(add));
  doc.lines?.forEach(add);
  return map;
}

/** Whether any line in the doc carries a redaction overlay (gates the marker filter). */
function docHasRedaction(doc: RenderDoc): boolean {
  let found = false;
  const check = (l?: RenderLine) => {
    if (l?.overlays?.some((o) => o.type === "redaction")) found = true;
  };
  doc.slides?.forEach((s) => {
    check(s.title);
    s.bullets.forEach(check);
  });
  (doc.sheets ?? (doc.sheet ? [doc.sheet] : [])).forEach((sh) => sh.cells.forEach(check));
  doc.paragraphs?.forEach(check);
  doc.pages?.forEach((p) => p.lines.forEach(check));
  doc.sections?.forEach((s) => s.lines.forEach(check));
  doc.lines?.forEach(check);
  return found;
}

const TOKEN_RE = /(\s+|[^\s]+)/g;
function tokenize(text: string): string[] {
  return text.match(TOKEN_RE) ?? [];
}

type DiffSpan = { text: string; changed: boolean };

function diffSpans(next: string, prev: string | undefined): DiffSpan[] {
  if (prev === undefined || prev === next) return [{ text: next, changed: false }];
  const a = tokenize(prev);
  const b = tokenize(next);
  const spans: DiffSpan[] = [];
  for (let i = 0; i < b.length; i++) {
    const tok = b[i];
    const isSpace = /^\s+$/.test(tok);
    const changed = !isSpace && a[i] !== tok;
    const last = spans[spans.length - 1];
    if (last && last.changed === changed) last.text += tok;
    else spans.push({ text: tok, changed });
  }
  return spans;
}

// ── Slot-text roll (transition="slot") ───────────────────────────────────────

// The hero "writing" effect: each line visibly rolls from its previous value to
// the new one (source → pseudo → Japanese) via slot-text. slot-text only rolls
// on text changes *after* mount, so SlotLine mounts showing `from` and, after an
// optional stagger `delay`, sets `target` to trigger the roll. It renders plain
// text (no overlay segmentation) — the stages that opt into slot carry no
// annotations.
const SLOT_OPTIONS: SlotOptions = { stagger: 24, duration: 280 };

// A deterministic, same-length letter scramble of `s` (spaces preserved) — the
// "from" value that lets a term decode/roll into place when it's annotated.
const SCRAMBLE_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
function scramble(s: string): string {
  let out = "";
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (/\s/.test(c)) {
      out += c;
      continue;
    }
    out += SCRAMBLE_CHARS[(s.charCodeAt(i) * 7 + i * 31) % SCRAMBLE_CHARS.length];
  }
  return out;
}

function SlotLine({
  from,
  target,
  delay = 0,
}: {
  from: string;
  target: string;
  delay?: number;
}): React.ReactElement {
  const [text, setText] = React.useState(from);
  const started = React.useRef(false);
  React.useEffect(() => {
    if (!started.current) {
      started.current = true;
      if (from === target) return; // nothing to roll
      const t = setTimeout(() => setText(target), Math.max(0, delay));
      return () => clearTimeout(t);
    }
    setText(target);
  }, [from, target, delay]);
  return <SlotText text={text} options={SLOT_OPTIONS} />;
}

// ── Redaction censor bar ─────────────────────────────────────────────────────

// The marker-blackout filter, reused verbatim from the RedactionDiagram: an SVG
// turbulence + displacement map gives a redaction bar rough, felt-tip edges
// (no JS, SSR-safe). Rendered once per preview that actually carries a redaction;
// the .censor class references it via filter: url(#REDACT_FILTER_ID).
const REDACT_FILTER_ID = "kapi-redact-marker";

function RedactMarkerFilter(): React.ReactElement {
  return (
    <svg className={styles.svgDefs} aria-hidden="true" focusable="false" width="0" height="0">
      <filter id={REDACT_FILTER_ID} x="-10%" y="-30%" width="120%" height="160%">
        <feTurbulence
          type="fractalNoise"
          baseFrequency="0.018 0.035"
          numOctaves={2}
          seed={2}
          result="noise"
        />
        <feDisplacementMap
          in="SourceGraphic"
          in2="noise"
          scale={1.8}
          xChannelSelector="R"
          yChannelSelector="G"
        />
      </filter>
    </svg>
  );
}

// ── Leaf text renderer: side + transition + overlays + diff ──────────────────

/**
 * LineText renders one RenderLine's active-side text, applying (in order): the
 * source→target transition (typewriter reveals a growing prefix; slot rolls the
 * whole line), overlay highlights for the visible text, and a before/after word
 * diff when no overlays apply to a token.
 */
function LineText({ line, seq = 0 }: { line: RenderLine; seq?: number }): React.ReactElement {
  const ctx = useCtx();
  const isSource = ctx.side === "source";
  // A block with no translation yet reads as its source rather than as a hole
  // in the document — a partially translated document is still a document, and
  // it is the block's status, not a gap, that says the target is outstanding.
  const target = isSource ? undefined : line.targets?.[ctx.side];
  const fullText = target || line.text;
  // The codes belong to whichever side's text is being read, so a target that
  // fell back to its source shows the source's codes rather than none.
  const codes = (target ? line.targetCodes?.[ctx.side] : line.codes) ?? [];

  // Remember the previously-rendered text so a slot roll can start from it.
  const prevFullText = React.useRef(fullText);
  React.useEffect(() => {
    prevFullText.current = fullText;
  }, [fullText]);

  const { visible, done, cycle } = useTextTransition(fullText, {
    effect: ctx.transition,
    granularity: ctx.typewriter,
    delay: ctx.stagger > 0 ? ctx.stagger * seq : 0,
    reducedMotion: ctx.reducedMotion,
  });

  // Resolve overlays against the *visible* prefix so highlights appear as the
  // typewriter reveals the words they cover.
  const spans = useMemo<ResolvedSpan[]>(() => {
    if (!ctx.annotations) return [];
    return resolveOverlaySpans(line.overlays, ctx.side, visible, ctx.overlayFilter);
  }, [ctx.annotations, ctx.side, ctx.overlayFilter, line.overlays, visible]);

  const segments = useMemo<TextSegment[]>(() => segmentText(visible, spans), [visible, spans]);

  // before/after diff only when there are no overlays (overlays own the styling)
  // and no codes (which split the text into pieces the word diff can't align).
  const prev = spans.length === 0 && codes.length === 0 ? ctx.beforeIndex?.get(line.id) : undefined;

  // A redacted line is never rolled — slot-text would briefly expose the cleartext
  // as it rolls. It falls through to the overlay path, which paints a censor bar.
  const hasRedaction = spans.some((s) => s.type === "redaction");

  // A content memory-leveraged line: rolls into its memory match wrapped in a "from memory"
  // highlight. The "tm" overlay is a line-level marker (not a rendered span).
  const memoryLine =
    !!ctx.annotations &&
    (line.overlays?.some((o) => o.type === "tm" && o.side === ctx.side) ?? false);

  // The rendered text's own direction. Stated on the inline span (not only on
  // the block element) because several kinds — slides, sheet cells, entry rows —
  // place LineText straight into a container the host owns. A `dir` attribute
  // also makes the element a bidi *isolate* per HTML's suggested rendering, so
  // an RTL cell can sit in an LTR table (or an LTR key beside an RTL value)
  // without either reordering the other.
  const attrs = lineDirAttrs(line, ctx);

  // Slot roll: render the line via slot-text, starting from the previous value.
  // It rolls one string, so a line's inline codes are not shown while it rolls —
  // the stages that opt into slot carry no annotations either.
  // (Reduced motion and redacted lines fall through to the instant text path.)
  if (ctx.transition === "slot" && !ctx.reducedMotion && !hasRedaction) {
    const roll = (
      <SlotLine
        from={prevFullText.current}
        target={fullText}
        delay={ctx.stagger > 0 ? ctx.stagger * seq : 0}
      />
    );
    return (
      <span {...attrs} className={cn(memoryLine && styles.memoryHit)}>
        {roll}
      </span>
    );
  }

  const showCaret = ctx.transition === "typewriter" && !done && !ctx.reducedMotion;
  const fadeKey = ctx.transition === "crossfade" ? cycle : undefined;

  return (
    <span
      key={fadeKey}
      {...attrs}
      className={cn(
        ctx.transition === "crossfade" && styles.fade,
        showCaret && styles.caret,
        memoryLine && styles.memoryHit,
      )}
    >
      {inlineNodes(segments, codes, visible.length, prev)}
    </span>
  );
}

/**
 * Weave a line's inline codes back into its overlay segments, in reading order.
 *
 * The two segmentations are independent — overlays cover ranges of text, codes
 * sit *between* characters — so a code inside an overlay splits that segment
 * into two marks around the chip rather than dropping either. `limit` is how far
 * the text has been revealed (the typewriter's visible prefix), so a code
 * appears only once its position has been reached.
 */
function inlineNodes(
  segments: TextSegment[],
  codes: InlineCode[],
  limit: number,
  prev: string | undefined,
): React.ReactNode[] {
  const nodes: React.ReactNode[] = [];
  let ci = 0;
  let pos = 0;

  const emitCodesAt = (offset: number) => {
    while (ci < codes.length && codes[ci].offset <= offset) {
      nodes.push(<InlineCodeChip key={`c${ci}`} code={codes[ci]} />);
      ci++;
    }
  };
  const emitText = (seg: TextSegment, text: string) => {
    if (!text) return;
    const key = `t${nodes.length}`;
    nodes.push(
      seg.overlay ? (
        <OverlayMark key={key} segment={{ text, overlay: seg.overlay }} />
      ) : (
        <DiffText key={key} text={text} prev={prev} />
      ),
    );
  };

  for (const seg of segments) {
    const start = pos;
    const end = pos + seg.text.length;
    let cursor = start;
    while (ci < codes.length && codes[ci].offset < end) {
      const at = Math.max(codes[ci].offset, start);
      emitText(seg, seg.text.slice(cursor - start, at - start));
      cursor = at;
      emitCodesAt(at);
    }
    emitText(seg, seg.text.slice(cursor - start));
    pos = end;
  }
  // Codes trailing the last character (e.g. a closing tag at the end of a line).
  emitCodesAt(limit);
  return nodes;
}

/**
 * One inline code as a chip: the vocabulary's short label in its type color,
 * with the original markup on the tooltip. Deliberately the same pill the
 * editor's cell renderers draw, so a placeholder reads identically wherever it
 * is shown — a source that quietly lost its `{{name}}` reads as a translation
 * that invented one.
 */
function InlineCodeChip({ code }: { code: InlineCode }): React.ReactElement {
  const colors = tagColors(code.span);
  return (
    <span
      className={styles.inlineCode}
      style={{ backgroundColor: colors.bg, borderColor: colors.border, color: colors.text }}
      title={semanticTooltip(code.span)}
      data-inline-code={code.span.span_type}
      dir="ltr"
    >
      {semanticLabel(code.span)}
    </span>
  );
}

function OverlayMark({ segment }: { segment: TextSegment }): React.ReactElement {
  const ctx = useCtx();
  const ov = segment.overlay!;
  // Redaction renders as a marker censor bar (the RedactionDiagram blackout): the
  // cleartext stays in the DOM for layout/width but is hidden under the ink bar
  // and masked from selection + assistive tech.
  if (ov.type === "redaction") {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <mark className={styles.censor} data-overlay-type="redaction" aria-label={ov.tooltip}>
            {segment.text}
          </mark>
        </TooltipTrigger>
        <TooltipContent>{ov.tooltip}</TooltipContent>
      </Tooltip>
    );
  }
  // A term decodes/rolls into place as its highlight sweeps in (the annotation
  // "effect"). slot-text rolls from a same-length scramble to the term.
  const content =
    ov.type === "term" && !ctx.reducedMotion ? (
      <SlotLine from={scramble(segment.text)} target={segment.text} />
    ) : (
      segment.text
    );
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <mark className={cn(styles.overlay, ov.style.className)} data-overlay-type={ov.type}>
          {content}
        </mark>
      </TooltipTrigger>
      <TooltipContent>{ov.tooltip}</TooltipContent>
    </Tooltip>
  );
}

function DiffText({ text, prev }: { text: string; prev: string | undefined }): React.ReactElement {
  // Note: word diff against the whole previous line is approximate when text is a
  // segment, but segments only split on overlays — when prev is set there are no
  // overlays, so `text` is the whole line and the diff is exact.
  const spans = useMemo(() => diffSpans(text, prev), [text, prev]);
  return (
    <>
      {spans.map((s, i) =>
        s.changed ? (
          <mark key={i} className={styles.changed}>
            {s.text}
          </mark>
        ) : (
          <React.Fragment key={i}>{s.text}</React.Fragment>
        ),
      )}
    </>
  );
}

// ── Structure renderers ──────────────────────────────────────────────────────

function SlideTitle({ line }: { line: RenderLine }): React.ReactElement {
  // dir/lang belong on this block-level element, not only on LineText's inline
  // span: text-align and the direction the browser lays the line out in follow
  // the nearest block/flex-item box's own `direction`, which an inline
  // descendant's `dir` does not reach.
  const ctx = useCtx();
  const attrs = lineDirAttrs(line, ctx);
  return (
    <div {...attrs} {...useBlockProps(line.id, styles.slideTitle)}>
      <LineText line={line} seq={0} />
    </div>
  );
}

function SlideBullet({ line, seq }: { line: RenderLine; seq: number }): React.ReactElement {
  const ctx = useCtx();
  const attrs = lineDirAttrs(line, ctx);
  return (
    <li {...attrs} {...useBlockProps(line.id)}>
      <LineText line={line} seq={seq} />
    </li>
  );
}

function Slides({ slides }: { slides: RenderSlide[] }): React.ReactElement {
  return (
    <div className={styles.slideDeck}>
      {slides.map((slide) => (
        <div key={slide.name} className={styles.slide}>
          {slide.title && <SlideTitle line={slide.title} />}
          {slide.bullets.length > 0 && (
            <ul className={styles.slideBullets}>
              {slide.bullets.map((b, i) => (
                <SlideBullet key={b.id} line={b} seq={i + 1} />
              ))}
            </ul>
          )}
        </div>
      ))}
    </div>
  );
}

function Cell({ cell }: { cell: RenderCell }): React.ReactElement {
  const ctx = useCtx();
  const attrs = lineDirAttrs(cell, ctx);
  return (
    <td {...attrs} {...useBlockProps(cell.id, styles.cell)}>
      <LineText line={cell} />
    </td>
  );
}

function Sheet({
  sheet,
  gridHeaders,
}: {
  sheet: RenderSheet;
  gridHeaders: boolean;
}): React.ReactElement {
  const grid: (RenderCell | null)[][] = [];
  for (let r = 0; r < sheet.rows; r++) {
    grid.push(Array.from<RenderCell | null>({ length: sheet.cols }).fill(null));
  }
  for (const c of sheet.cells) {
    if (grid[c.row] && c.col < sheet.cols) grid[c.row][c.col] = c;
  }
  return (
    <div className={styles.sheetWrap}>
      <table className={styles.sheet}>
        <tbody>
          {gridHeaders && (
            <tr className={styles.colHead}>
              <td className={styles.corner} aria-hidden="true" />
              {Array.from({ length: sheet.cols }, (_, c) => (
                <td key={c}>{colLabel(c)}</td>
              ))}
            </tr>
          )}
          {grid.map((row, r) => (
            <tr key={r}>
              {gridHeaders && <td className={styles.rowHead}>{r + 1}</td>}
              {row.map((cell, c) =>
                cell ? <Cell key={c} cell={cell} /> : <td key={c} className={styles.cell} />,
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/**
 * Render one line's block element. Split from `lineEl` so it can read the
 * preview context (writing direction) — `lineEl` stays a plain mapper usable
 * directly in `.map(lineEl)`.
 */
function Line({ line, index = 0 }: { line: RenderLine; index?: number }): React.ReactElement {
  const ctx = useCtx();
  const attrs = lineDirAttrs(line, ctx);
  const ownClass =
    line.role === "code"
      ? styles.code
      : line.role === "heading"
        ? styles.heading
        : line.role === "bullet"
          ? undefined
          : cn(styles.para, line.preserveWhitespace && styles.preserve);
  const blockProps = useBlockProps(line.id, ownClass);
  const content = <LineText line={line} seq={index} />;

  // Code is preformatted, monospaced, and never typeset as prose: markdown
  // markers inside it are literal characters, and its whitespace is meaningful.
  if (line.role === "code") {
    return (
      <pre {...attrs} {...blockProps} data-code-language={line.codeLanguage}>
        <code>{content}</code>
      </pre>
    );
  }
  if (line.role === "heading") {
    return (
      <div {...attrs} {...blockProps}>
        {content}
      </div>
    );
  }
  // The list is the typographic frame; the item is the block, so the selection
  // affordance sits on the <li> rather than the <ul> around it.
  if (line.role === "bullet") {
    return (
      <ul {...attrs} className={styles.bulletList}>
        <li {...blockProps}>{content}</li>
      </ul>
    );
  }
  return (
    <p {...attrs} {...blockProps}>
      {content}
    </p>
  );
}

function lineEl(p: RenderLine, index = 0): React.ReactElement {
  return <Line key={p.id} line={p} index={index} />;
}

function Doc({ paragraphs }: { paragraphs: RenderLine[] }): React.ReactElement {
  return <div className={styles.page}>{paragraphs.map(lineEl)}</div>;
}

function Pages({ pages }: { pages: RenderPage[] }): React.ReactElement {
  return (
    <div className="flex flex-col gap-2">
      {pages.map((pg) => (
        <div key={pg.name} className={styles.page}>
          <div className={styles.pageLabel}>{pg.name}</div>
          {pg.lines.map(lineEl)}
        </div>
      ))}
    </div>
  );
}

function Entry({ line, seq }: { line: RenderLine; seq: number }): React.ReactElement {
  const ctx = useCtx();
  // dir/lang go on entryText, not the row (.entry is a flex row with the key
  // pinned to a fixed-width column): dir on the row itself would reverse flex
  // item order for an RTL value, swapping the always-LTR key to the far side
  // instead of just right-aligning the value's own text.
  const attrs = lineDirAttrs(line, ctx);
  return (
    <div {...useBlockProps(line.id, styles.entry)}>
      {line.key && (
        // A catalog key is an identifier, not prose: always LTR, and
        // isolated so it cannot be reordered by an adjacent RTL value.
        <bdi dir="ltr" className={styles.entryKey} title={line.key}>
          {line.key}
        </bdi>
      )}
      <span {...attrs} className={styles.entryText}>
        <LineText line={line} seq={seq} />
      </span>
    </div>
  );
}

function List({ lines }: { lines: RenderLine[] }): React.ReactElement {
  return (
    <div className={styles.list}>
      {lines.map((l, i) => (
        <Entry key={l.id} line={l} seq={i} />
      ))}
    </div>
  );
}

function Sections({ sections }: { sections: RenderSection[] }): React.ReactElement {
  return (
    <div className={styles.page}>
      {sections.map((sec) => (
        <div
          key={sec.name + sec.depth}
          className={styles.section}
          style={{ marginLeft: `${sec.depth * 0.75}rem` }}
        >
          <div className={styles.sectionTitle}>{sec.name}</div>
          {sec.lines.map(lineEl)}
        </div>
      ))}
    </div>
  );
}

// ── Body dispatch ────────────────────────────────────────────────────────────

function PreviewBody({
  doc,
  gridHeaders,
}: {
  doc: RenderDoc;
  gridHeaders: boolean;
}): React.ReactElement {
  switch (doc.kind) {
    case "slides":
      return <Slides slides={doc.slides ?? []} />;
    case "sheet":
      return doc.sheet ? (
        <Sheet sheet={doc.sheet} gridHeaders={gridHeaders} />
      ) : (
        <List lines={[]} />
      );
    case "doc":
      return <Doc paragraphs={doc.paragraphs ?? []} />;
    case "pages":
      return <Pages pages={doc.pages ?? []} />;
    case "sections":
      return <Sections sections={doc.sections ?? []} />;
    case "list":
    default:
      return <List lines={doc.lines ?? []} />;
  }
}

// ── Public component ─────────────────────────────────────────────────────────

export default function FormatPreview({
  tree,
  doc,
  before,
  side = "source",
  annotations = true,
  flush = false,
  overlayTypes,
  transition = "none",
  typewriter = "word",
  typewriterStagger = 0,
  reducedMotion,
  gridHeaders = true,
  onSelectBlock,
  selectedBlockId,
  blockAttrs,
  className,
}: FormatPreviewProps): React.ReactElement {
  const model = useMemo<RenderDoc>(() => {
    if (doc) return doc;
    if (tree) return treeToRenderDoc(tree);
    return { kind: "list", format: "", locales: [], lines: [] };
  }, [doc, tree]);

  const beforeIndex = useMemo(() => indexById(before), [before]);
  const overlayFilter = useMemo(
    () => (overlayTypes ? new Set(overlayTypes) : undefined),
    [overlayTypes],
  );
  const needsMarker = useMemo(() => annotations && docHasRedaction(model), [annotations, model]);

  // When the engine ships a projected render AST (ContentTree.render) and the
  // call is the plain source preview — no explicit `doc`/`before` override and
  // no animation — render it faithfully (real inline formatting + reconstructed
  // tables, preview-fidelity #1/#2). Overlay-highlight / diff / typewriter
  // previews keep the structured RenderDoc path, which segments flat text.
  // A host that selects or decorates blocks needs each block on its own
  // element, which is the structured RenderDoc path — the projected render AST
  // is a faithful document, not an addressable one.
  const useProjection =
    !doc &&
    !before &&
    transition === "none" &&
    side === "source" &&
    !!tree?.render &&
    !onSelectBlock &&
    !blockAttrs;

  const ctx = useMemo<PreviewCtx>(
    () => ({
      side,
      annotations,
      overlayFilter,
      transition,
      typewriter,
      stagger: typewriterStagger,
      reducedMotion,
      beforeIndex,
      sourceLocale: model.sourceLocale,
      onSelectBlock,
      selectedBlockId,
      blockAttrs,
    }),
    [
      side,
      annotations,
      overlayFilter,
      transition,
      typewriter,
      typewriterStagger,
      reducedMotion,
      beforeIndex,
      model.sourceLocale,
      onSelectBlock,
      selectedBlockId,
      blockAttrs,
    ],
  );

  return (
    <TooltipProvider delayDuration={150}>
      <Ctx.Provider value={ctx}>
        <div className={cn("kapi-reference", styles.root, flush && styles.flush, className)}>
          {needsMarker && <RedactMarkerFilter />}
          {useProjection && tree?.render ? (
            <RenderedDocument node={tree.render} locale={model.sourceLocale} />
          ) : (
            <PreviewBody doc={model} gridHeaders={gridHeaders} />
          )}
        </div>
      </Ctx.Provider>
    </TooltipProvider>
  );
}
