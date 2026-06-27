import React, { useMemo } from "react";
import { cn } from "../../lib/utils";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "../ui/tooltip";
import type {
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
}

const Ctx = React.createContext<PreviewCtx | null>(null);

function useCtx(): PreviewCtx {
  const c = React.useContext(Ctx);
  if (!c) throw new Error("FormatPreview line rendered outside provider");
  return c;
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
  const fullText = isSource ? line.text : (line.targets?.[ctx.side] ?? line.text);

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

  // before/after diff only when there are no overlays (overlays own the styling).
  const prev = spans.length === 0 ? ctx.beforeIndex?.get(line.id) : undefined;

  // A redacted line is never rolled — slot-text would briefly expose the cleartext
  // as it rolls. It falls through to the overlay path, which paints a censor bar.
  const hasRedaction = spans.some((s) => s.type === "redaction");

  // A TM-leveraged line: rolls into its memory match wrapped in a "from memory"
  // highlight. The "tm" overlay is a line-level marker (not a rendered span).
  const tmLine =
    !!ctx.annotations &&
    (line.overlays?.some((o) => o.type === "tm" && o.side === ctx.side) ?? false);

  // Slot roll: render the line via slot-text, starting from the previous value.
  // (Reduced motion and redacted lines fall through to the instant text path.)
  if (ctx.transition === "slot" && !ctx.reducedMotion && !hasRedaction) {
    const roll = (
      <SlotLine
        from={prevFullText.current}
        target={fullText}
        delay={ctx.stagger > 0 ? ctx.stagger * seq : 0}
      />
    );
    return tmLine ? <span className={styles.tmHit}>{roll}</span> : roll;
  }

  const showCaret = ctx.transition === "typewriter" && !done && !ctx.reducedMotion;
  const fadeKey = ctx.transition === "crossfade" ? cycle : undefined;

  return (
    <span
      key={fadeKey}
      className={cn(
        ctx.transition === "crossfade" && styles.fade,
        showCaret && styles.caret,
        tmLine && styles.tmHit,
      )}
    >
      {segments.map((seg, i) =>
        seg.overlay ? (
          <OverlayMark key={i} segment={seg} />
        ) : (
          <DiffText key={i} text={seg.text} prev={prev} />
        ),
      )}
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

function Slides({ slides }: { slides: RenderSlide[] }): React.ReactElement {
  return (
    <div className={styles.slideDeck}>
      {slides.map((slide) => (
        <div key={slide.name} className={styles.slide}>
          {slide.title && (
            <div className={styles.slideTitle}>
              <LineText line={slide.title} seq={0} />
            </div>
          )}
          {slide.bullets.length > 0 && (
            <ul className={styles.slideBullets}>
              {slide.bullets.map((b, i) => (
                <li key={b.id}>
                  <LineText line={b} seq={i + 1} />
                </li>
              ))}
            </ul>
          )}
        </div>
      ))}
    </div>
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
              {row.map((cell, c) => (
                <td key={c} className={styles.cell}>
                  {cell ? <LineText line={cell} /> : ""}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function lineEl(p: RenderLine, index = 0): React.ReactElement {
  const content = <LineText line={p} seq={index} />;
  if (p.role === "heading") {
    return (
      <div key={p.id} className={styles.heading}>
        {content}
      </div>
    );
  }
  if (p.role === "bullet") {
    return (
      <ul key={p.id} className={styles.bulletList}>
        <li>{content}</li>
      </ul>
    );
  }
  return (
    <p key={p.id} className={styles.para}>
      {content}
    </p>
  );
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

function List({ lines }: { lines: RenderLine[] }): React.ReactElement {
  return (
    <div className={styles.list}>
      {lines.map((l, i) => (
        <div key={l.id} className={styles.entry}>
          {l.key && (
            <span className={styles.entryKey} title={l.key}>
              {l.key}
            </span>
          )}
          <span className={styles.entryText}>
            <LineText line={l} seq={i} />
          </span>
        </div>
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
  const useProjection =
    !doc && !before && transition === "none" && side === "source" && !!tree?.render;

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
    ],
  );

  return (
    <TooltipProvider delayDuration={150}>
      <Ctx.Provider value={ctx}>
        <div className={cn("kapi-reference", styles.root, flush && styles.flush, className)}>
          {needsMarker && <RedactMarkerFilter />}
          {useProjection && tree?.render ? (
            <RenderedDocument node={tree.render} />
          ) : (
            <PreviewBody doc={model} gridHeaders={gridHeaders} />
          )}
        </div>
      </Ctx.Provider>
    </TooltipProvider>
  );
}
