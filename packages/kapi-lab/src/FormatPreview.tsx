import React, { useMemo } from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
  cn,
} from "@neokapi/ui-primitives";
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
import styles from "./FormatPreview.module.css";

// FormatPreview — a structure-aware, annotation-aware preview that renders ANY
// neokapi format from a ContentTree (or a normalized RenderDoc). The structural
// shape (slides / sheet / pages / doc / list / sections) is decided by
// treeToRenderDoc's data-driven dispatch, so a new format degrades gracefully.
//
// It renders either the source runs or a chosen target locale's runs, highlights
// each block's stand-off overlays (color-coded by type, with a tooltip), and
// animates the source→target swap with a crossfade or typewriter effect
// (honoring prefers-reduced-motion).

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
  /** Restrict overlay highlights to these types (undefined = all). */
  overlayTypes?: string[];
  /** Source→target transition effect (default "none"). */
  transition?: TransitionEffect;
  /** Typewriter granularity when transition === "typewriter" (default "word"). */
  typewriter?: TypewriterGranularity;
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

// ── Leaf text renderer: side + transition + overlays + diff ──────────────────

/**
 * LineText renders one RenderLine's active-side text, applying (in order): the
 * source→target transition (typewriter reveals a growing prefix), overlay
 * highlights for the visible text, and a before/after word diff when no overlays
 * apply to a token.
 */
function LineText({ line }: { line: RenderLine }): React.ReactElement {
  const ctx = useCtx();
  const isSource = ctx.side === "source";
  const fullText = isSource ? line.text : (line.targets?.[ctx.side] ?? line.text);

  const { visible, done, cycle } = useTextTransition(fullText, {
    effect: ctx.transition,
    granularity: ctx.typewriter,
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

  const showCaret = ctx.transition === "typewriter" && !done && !ctx.reducedMotion;
  const fadeKey = ctx.transition === "crossfade" ? cycle : undefined;

  return (
    <span
      key={fadeKey}
      className={cn(ctx.transition === "crossfade" && styles.fade, showCaret && styles.caret)}
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
  const ov = segment.overlay!;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <mark className={cn(styles.overlay, ov.style.className)} data-overlay-type={ov.type}>
          {segment.text}
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
              <LineText line={slide.title} />
            </div>
          )}
          {slide.bullets.length > 0 && (
            <ul className={styles.slideBullets}>
              {slide.bullets.map((b) => (
                <li key={b.id}>
                  <LineText line={b} />
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

function lineEl(p: RenderLine): React.ReactElement {
  const content = <LineText line={p} />;
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
      {lines.map((l) => (
        <div key={l.id} className={styles.entry}>
          {l.key && (
            <span className={styles.entryKey} title={l.key}>
              {l.key}
            </span>
          )}
          <span className={styles.entryText}>
            <LineText line={l} />
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
  overlayTypes,
  transition = "none",
  typewriter = "word",
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

  const ctx = useMemo<PreviewCtx>(
    () => ({
      side,
      annotations,
      overlayFilter,
      transition,
      typewriter,
      reducedMotion,
      beforeIndex,
    }),
    [side, annotations, overlayFilter, transition, typewriter, reducedMotion, beforeIndex],
  );

  return (
    <TooltipProvider delayDuration={150}>
      <Ctx.Provider value={ctx}>
        <div className={cn("kapi-reference", styles.root, className)}>
          <PreviewBody doc={model} gridHeaders={gridHeaders} />
        </div>
      </Ctx.Provider>
    </TooltipProvider>
  );
}
