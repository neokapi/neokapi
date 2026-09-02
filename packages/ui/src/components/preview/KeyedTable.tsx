// KeyedTable — a catalog-keyvalue document read as what it is: units addressed
// by a key path, one row each, grouped the way the file nests.
//
// The document reading renders the same file as a column of strings. For prose
// that is right; for `messages.json` it leaves a reviewer looking at French
// with nothing to say which string is the button and which is the error, and
// with the source nowhere in sight. Here the key is a column, the source sits
// beside the target, and `errors.network.timeout` reads under `errors` then
// `network`.
//
// The row carries the same `data-block-id` the document preview puts on a
// block, and the same host decoration through `blockAttrs`, so selection and
// scroll-into-view work in a host that knows nothing about which preview drew
// the element.

import React from "react";
import { cn } from "../../lib/utils";
import { DirectionalText } from "../../lib/text-direction";
import { useBlockElementProps, type BlockAttrs } from "./blockElement";
import { OverlayText, weaveInline } from "./inlineContent";
import { groupRows, keyedRows, type KeyedRow, type KeyGroup } from "./keyModel";
import { resolveOverlaySpans, segmentText } from "./overlayHighlight";
import { runsCodes, runsText } from "./renderDoc";
import type { ContentTree, OverlayView, Run } from "./types";
import styles from "./KeyedTable.module.css";

export interface KeyedTableProps {
  /** The document to read. */
  tree: ContentTree;
  /**
   * The target locale in view. Present, the table carries a target column
   * beside the source; absent, it shows the source alone.
   */
  locale?: string;
  /** The document's source locale, for the source column's writing direction. */
  sourceLocale?: string;
  /** Called with a block's id when its row is activated (click, Enter, Space). */
  onSelectBlock?: (id: string) => void;
  /** The row in focus, marked `aria-current` and scrolled into view by the host. */
  selectedBlockId?: string;
  /** Per-block class name and `data-*` markers. */
  blockAttrs?: (id: string) => BlockAttrs | undefined;
  /** Column header for the source side (default: the source locale, else "Source"). */
  sourceLabel?: string;
  /** Column header for the target side (default: the locale). */
  targetLabel?: string;
  className?: string;
}

// ── The run reading ──────────────────────────────────────────────────────────

/**
 * One side of a unit: its text, the inline codes positioned in it, and the
 * evidence marked over it.
 *
 * The three come from the same declared projection the document reading uses
 * (`runsText` and `runsCodes` over renderDoc's DOCUMENT_PIECES spec), so a
 * placeholder, a paired code, a plural's pivot and a break render here exactly
 * as they render there, and an overlay's offsets mean the same thing in both.
 * A cell is where a run sequence is most easily flattened into "the text", and
 * flattening is what leaves a placeholder invisible and two lines running
 * together.
 */
function RunCell({
  runs,
  overlays,
  side,
  locale,
}: {
  runs: Run[] | undefined;
  overlays: OverlayView[] | undefined;
  side: string;
  locale?: string;
}): React.ReactElement {
  const text = React.useMemo(() => runsText(runs), [runs]);
  const codes = React.useMemo(() => runsCodes(runs), [runs]);
  const segments = React.useMemo(() => {
    const spans = overlays ? resolveOverlaySpans(overlays, side, text) : [];
    return segmentText(text, spans);
  }, [overlays, side, text]);
  const nodes = React.useMemo(
    () =>
      weaveInline(segments, codes, text.length, (seg, value, key) => (
        <OverlayText
          key={key}
          segment={{ text: value, ...(seg.overlay ? { overlay: seg.overlay } : {}) }}
        />
      )),
    [segments, codes, text.length],
  );

  return (
    <DirectionalText as="span" locale={locale} className="whitespace-pre-wrap">
      {nodes}
    </DirectionalText>
  );
}

// ── The table ────────────────────────────────────────────────────────────────

/** One unit's row: the key, the source, and the target when one is in view. */
function Row({
  row,
  locale,
  sourceLocale,
  columns,
  ctx,
}: {
  row: KeyedRow;
  locale?: string;
  sourceLocale?: string;
  columns: number;
  ctx: Pick<KeyedTableProps, "onSelectBlock" | "selectedBlockId" | "blockAttrs">;
}): React.ReactElement {
  const props = useBlockElementProps({
    id: row.id,
    attrs: ctx.blockAttrs?.(row.id),
    selected: ctx.selectedBlockId === row.id,
    ...(ctx.onSelectBlock ? { onSelect: ctx.onSelectBlock } : {}),
    className: styles.row,
    selectableClass: styles.selectable,
    selectedClass: styles.selected,
  });
  const { className, ...rest } = props;

  return (
    <tr {...rest} className={className} data-key-path={row.key}>
      <th
        scope="row"
        className="w-[1%] max-w-[18rem] truncate border-b border-border py-1.5 pr-4 pl-2 text-left align-top font-mono text-[11px] font-normal text-muted-foreground"
        title={row.key}
      >
        <bdi dir="ltr">{row.leaf}</bdi>
      </th>
      <td className="border-b border-border py-1.5 pr-4 align-top text-[0.8rem]">
        <RunCell
          runs={row.source}
          overlays={row.node.overlays}
          side="source"
          locale={row.sourceLocale ?? sourceLocale}
        />
      </td>
      {columns > 2 && locale && (
        <td className="border-b border-border py-1.5 pr-2 align-top text-[0.8rem]">
          <RunCell
            runs={row.targets[locale]}
            overlays={row.node.overlays}
            side={locale}
            locale={locale}
          />
        </td>
      )}
    </tr>
  );
}

/** A group's heading row and everything under it, deepest groups last. */
function Group({
  group,
  depth,
  columns,
  locale,
  sourceLocale,
  ctx,
}: {
  group: KeyGroup;
  depth: number;
  columns: number;
  locale?: string;
  sourceLocale?: string;
  ctx: Pick<KeyedTableProps, "onSelectBlock" | "selectedBlockId" | "blockAttrs">;
}): React.ReactElement {
  const heading = group.path.length > 0;
  return (
    <>
      {heading && (
        <tr data-group-path={group.path.join(".")}>
          <th
            scope="colgroup"
            colSpan={columns}
            className="border-b border-border bg-muted/40 py-1 pr-2 text-left align-middle font-mono text-[11px] font-medium text-foreground"
            style={{ paddingInlineStart: `${0.5 + depth * 0.85}rem` }}
          >
            <bdi dir="ltr">{group.path.join(" › ")}</bdi>
          </th>
        </tr>
      )}
      {group.rows.map((row) => (
        <Row
          key={row.id}
          row={row}
          locale={locale}
          sourceLocale={sourceLocale}
          columns={columns}
          ctx={ctx}
        />
      ))}
      {group.groups.map((child) => (
        <Group
          key={child.path.join(".")}
          group={child}
          depth={depth + 1}
          columns={columns}
          locale={locale}
          sourceLocale={sourceLocale}
          ctx={ctx}
        />
      ))}
    </>
  );
}

export default function KeyedTable({
  tree,
  locale,
  sourceLocale,
  onSelectBlock,
  selectedBlockId,
  blockAttrs,
  sourceLabel,
  targetLabel,
  className,
}: KeyedTableProps): React.ReactElement {
  const rows = React.useMemo(() => keyedRows(tree), [tree]);
  const root = React.useMemo(() => groupRows(rows), [rows]);
  const columns = locale ? 3 : 2;
  const ctx = { onSelectBlock, selectedBlockId, blockAttrs };

  if (rows.length === 0) {
    return (
      <p className={cn("p-6 text-center text-muted-foreground", className)}>
        This file holds no units.
      </p>
    );
  }

  return (
    <div className={cn("overflow-x-auto", className)} data-preview="keyed-table">
      <table className="w-full border-collapse text-left">
        <thead>
          <tr className="border-b border-border">
            <th
              scope="col"
              className="py-1.5 pr-4 pl-2 text-[11px] font-medium tracking-wide text-muted-foreground uppercase"
            >
              Key
            </th>
            <th
              scope="col"
              className="py-1.5 pr-4 text-[11px] font-medium tracking-wide text-muted-foreground uppercase"
            >
              {sourceLabel ?? sourceLocale ?? "Source"}
            </th>
            {columns > 2 && (
              <th
                scope="col"
                className="py-1.5 pr-2 text-[11px] font-medium tracking-wide text-muted-foreground uppercase"
              >
                {targetLabel ?? locale}
              </th>
            )}
          </tr>
        </thead>
        <tbody>
          <Group
            group={root}
            depth={0}
            columns={columns}
            {...(locale ? { locale } : {})}
            {...(sourceLocale ? { sourceLocale } : {})}
            ctx={ctx}
          />
        </tbody>
      </table>
    </div>
  );
}
