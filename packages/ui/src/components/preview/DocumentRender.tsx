import React, { useMemo } from "react";
import { cn } from "../../lib/utils";
import type { RenderCell, RenderDoc, RenderLine, RenderSheet, RenderSlide } from "./renderDoc";
import { colLabel } from "./renderDoc";
import styles from "./DocumentRender.module.css";

// DocumentRender — paint a RenderDoc (the normalized extraction model) as the
// document a human would recognize: a 16:9 slide deck, a spreadsheet grid, or a
// page of paragraphs. The hero carousel and the Try-Neokapi modal both render
// through this one component, so the instant teaser and the live-engine proof
// look identical.
//
// When a `before` model is supplied, each line/cell is diffed against its
// same-id counterpart and the changed words are highlighted with the site's
// green accent — so an "after" (translated / search-replaced) document shows
// exactly what the pipeline touched.

export interface DocumentRenderProps {
  doc: RenderDoc;
  /** The "before" model; when present, changed words are highlighted vs it. */
  before?: RenderDoc;
  /** Optional explicit set of changed block ids (overrides the before-diff). */
  changed?: Set<string>;
  className?: string;
  /** Show spreadsheet column letters / row numbers (default true). */
  gridHeaders?: boolean;
}

// ── Word-level diff highlight ────────────────────────────────────────────────

type Span = { text: string; changed: boolean };

// Split into words + the whitespace/punctuation between them, so a highlight
// wraps whole changed words rather than character runs.
const TOKEN_RE = /(\s+|[^\s]+)/g;

function tokenize(text: string): string[] {
  return text.match(TOKEN_RE) ?? [];
}

/**
 * Diff `next` against `prev` at word granularity, returning spans tagged as
 * changed where the words differ. When `prev` is undefined the whole string is
 * unchanged (a "before" render, or no comparison requested).
 */
function diffSpans(next: string, prev: string | undefined): Span[] {
  if (prev === undefined || prev === next) return [{ text: next, changed: false }];
  const a = tokenize(prev);
  const b = tokenize(next);
  // A light LCS-free heuristic: walk both, treating a word as "changed" when it
  // is not the identical token at the same position. This is more than enough for
  // the short marketing/extraction strings we render, and keeps the highlight
  // crisp (whole-word) without an O(n²) table.
  const spans: Span[] = [];
  const max = b.length;
  for (let i = 0; i < max; i++) {
    const tok = b[i];
    const isSpace = /^\s+$/.test(tok);
    const changed = !isSpace && a[i] !== tok;
    const last = spans[spans.length - 1];
    if (last && last.changed === changed) last.text += tok;
    else spans.push({ text: tok, changed });
  }
  return spans;
}

function HighlightedText({ next, prev }: { next: string; prev: string | undefined }) {
  const spans = useMemo(() => diffSpans(next, prev), [next, prev]);
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

// ── before-model lookup ──────────────────────────────────────────────────────

/** Build an id→text index over every line/cell in a RenderDoc, for diffing. */
function indexById(doc: RenderDoc | undefined): Map<string, string> | null {
  if (!doc) return null;
  const map = new Map<string, string>();
  const addLine = (l: RenderLine | undefined) => {
    if (l) map.set(l.id, l.text);
  };
  doc.slides?.forEach((s) => {
    addLine(s.title);
    s.bullets.forEach(addLine);
  });
  (doc.sheets ?? (doc.sheet ? [doc.sheet] : [])).forEach((sh) =>
    sh.cells.forEach((c) => map.set(c.id, c.text)),
  );
  doc.paragraphs?.forEach(addLine);
  doc.lines?.forEach(addLine);
  return map;
}

// ── Renderers per kind ───────────────────────────────────────────────────────

function lineBefore(
  id: string,
  beforeIndex: Map<string, string> | null,
  changed: Set<string> | undefined,
): string | undefined {
  // An explicit changed-set wins: a changed id has no matching "before" word, so
  // highlight the whole line by diffing against an empty string.
  if (changed) return changed.has(id) ? "" : undefined;
  return beforeIndex?.get(id);
}

function Slides({
  slides,
  beforeIndex,
  changed,
}: {
  slides: RenderSlide[];
  beforeIndex: Map<string, string> | null;
  changed: Set<string> | undefined;
}) {
  return (
    <div className={styles.slideDeck}>
      {slides.map((slide) => (
        <div key={slide.name} className={styles.slide}>
          {slide.title && (
            <div className={styles.slideTitle}>
              <HighlightedText
                next={slide.title.text}
                prev={lineBefore(slide.title.id, beforeIndex, changed)}
              />
            </div>
          )}
          {slide.bullets.length > 0 && (
            <ul className={styles.slideBullets}>
              {slide.bullets.map((b) => (
                <li key={b.id}>
                  <HighlightedText next={b.text} prev={lineBefore(b.id, beforeIndex, changed)} />
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
  beforeIndex,
  changed,
  gridHeaders,
}: {
  sheet: RenderSheet;
  beforeIndex: Map<string, string> | null;
  changed: Set<string> | undefined;
  gridHeaders: boolean;
}) {
  // Place cells into a dense (row × col) grid so blank cells render as empty.
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
                  {cell ? (
                    <HighlightedText
                      next={cell.text}
                      prev={lineBefore(cell.id, beforeIndex, changed)}
                    />
                  ) : (
                    ""
                  )}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Doc({
  paragraphs,
  beforeIndex,
  changed,
}: {
  paragraphs: RenderLine[];
  beforeIndex: Map<string, string> | null;
  changed: Set<string> | undefined;
}) {
  return (
    <div className={styles.page}>
      {paragraphs.map((p) => {
        const content = (
          <HighlightedText next={p.text} prev={lineBefore(p.id, beforeIndex, changed)} />
        );
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
      })}
    </div>
  );
}

function List({
  lines,
  beforeIndex,
  changed,
}: {
  lines: RenderLine[];
  beforeIndex: Map<string, string> | null;
  changed: Set<string> | undefined;
}) {
  return (
    <div className={styles.page}>
      {lines.map((l) => (
        <div key={l.id} className={styles.keyRow}>
          <HighlightedText next={l.text} prev={lineBefore(l.id, beforeIndex, changed)} />
        </div>
      ))}
    </div>
  );
}

export default function DocumentRender({
  doc,
  before,
  changed,
  className,
  gridHeaders = true,
}: DocumentRenderProps): React.ReactElement {
  const beforeIndex = useMemo(() => indexById(before), [before]);

  let body: React.ReactNode;
  if (doc.kind === "slides" && doc.slides) {
    body = <Slides slides={doc.slides} beforeIndex={beforeIndex} changed={changed} />;
  } else if (doc.kind === "sheet" && doc.sheet) {
    body = (
      <Sheet
        sheet={doc.sheet}
        beforeIndex={beforeIndex}
        changed={changed}
        gridHeaders={gridHeaders}
      />
    );
  } else if (doc.kind === "doc" && doc.paragraphs) {
    body = <Doc paragraphs={doc.paragraphs} beforeIndex={beforeIndex} changed={changed} />;
  } else {
    body = <List lines={doc.lines ?? []} beforeIndex={beforeIndex} changed={changed} />;
  }

  return <div className={cn(styles.root, className)}>{body}</div>;
}
