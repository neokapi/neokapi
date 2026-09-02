// DataPreview — the reading for a catalog-keyvalue document: a keyed table, and
// where the host can supply one, the written-back file beside it.
//
// The two views answer different questions. The table answers "what does this
// unit say, and which unit is it": keys in one column, source and target in the
// others, grouped by nesting. The code view answers "what will the file look
// like": the target locale's file as the format writer emits it, with the unit
// in focus marked.
//
// The code view is a capability the host supplies, not something this component
// can produce. The desktop has the source file on disk and the core writer, so
// it passes a `code` source. The platform stores blocks rather than files
// (bowrain/core/store.Item carries a block index, no source bytes and no
// skeleton), so it passes none and the toggle does not appear. Reconstructing a
// file from the blocks alone would be a guess shown as if it were the file:
// dotted key paths are ambiguous for a key containing a dot and for an array,
// and the writer's own formatting decisions are not recoverable from the units.

import React from "react";
import { cn } from "../../lib/utils";
import CodeView from "./CodeView";
import KeyedTable, { type KeyedTableProps } from "./KeyedTable";
import { keyedRows } from "./keyModel";
import type { BlockAttrs } from "./blockElement";
import type { ContentTree } from "./types";

/** Which reading the preview is showing. */
export type DataView = "table" | "code";

/**
 * The written-back file, supplied by a host that can produce one.
 *
 * `text` is the file as the format writer emits it for the side in view: the
 * target locale's file, or the source file when no locale is in view. A host
 * that loads it lazily passes `onRequest`, which the preview calls the first
 * time a reader opens the code view.
 */
export interface DataCodeSource {
  /** The serialized file. Absent while `loading`, or when `error` says why not. */
  text?: string;
  /** The file name, for the highlighter's language detection. */
  filename?: string;
  /** True while the host is fetching it. */
  loading?: boolean;
  /** Why the file could not be produced, shown in place of it. */
  error?: string;
  /** Called when a reader opens the code view and no text is loaded. */
  onRequest?: () => void;
}

export interface DataPreviewProps extends Pick<
  KeyedTableProps,
  "tree" | "locale" | "sourceLocale" | "onSelectBlock" | "selectedBlockId" | "blockAttrs"
> {
  /** The written-back file. Absent, the preview shows the table alone. */
  code?: DataCodeSource;
  /** Which view to show. Uncontrolled when absent. */
  view?: DataView;
  /** Called when the reader switches views. */
  onViewChange?: (view: DataView) => void;
  className?: string;
}

/**
 * The lines of `text` holding the unit in focus.
 *
 * The unit is found by its key, which is what a catalog file writes beside the
 * value on the same line. A key the file does not contain verbatim marks
 * nothing rather than the wrong line.
 */
export function linesForKey(text: string, key: string | undefined): Set<number> {
  const out = new Set<number>();
  if (!text || !key) return out;
  const leaf = key.split(/[./]/).filter(Boolean).pop() ?? key;
  const needles = [key, leaf].filter((n) => n.length > 0);
  const lines = text.split("\n");
  for (let i = 0; i < lines.length; i++) {
    if (needles.some((n) => lines[i].includes(n))) {
      out.add(i);
      // The first line carrying the deepest name is the entry; a key repeated
      // deeper in the file belongs to another entry and is left unmarked.
      if (lines[i].includes(needles[0])) break;
    }
  }
  return out;
}

function ViewTab({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        "rounded px-2 py-0.5 text-[11px] font-medium transition-colors",
        active
          ? "bg-background text-foreground shadow-sm"
          : "text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}

export default function DataPreview({
  tree,
  locale,
  sourceLocale,
  onSelectBlock,
  selectedBlockId,
  blockAttrs,
  code,
  view,
  onViewChange,
  className,
}: DataPreviewProps): React.ReactElement {
  const [ownView, setOwnView] = React.useState<DataView>("table");
  const active = view ?? ownView;
  const hasCode = code !== undefined;

  const setView = React.useCallback(
    (next: DataView) => {
      setOwnView(next);
      onViewChange?.(next);
      if (next === "code" && code && code.text === undefined && !code.loading) code.onRequest?.();
    },
    [onViewChange, code],
  );

  // The key of the unit in focus, so the code view can mark its line.
  const focusKey = React.useMemo(() => {
    if (!selectedBlockId) return undefined;
    return keyedRows(tree).find((row) => row.id === selectedBlockId)?.key;
  }, [tree, selectedBlockId]);

  const changed = React.useMemo(
    () => linesForKey(code?.text ?? "", focusKey),
    [code?.text, focusKey],
  );

  return (
    <div className={cn("flex min-h-0 flex-col", className)} data-preview="data">
      {hasCode && (
        <div className="mb-2 flex items-center gap-1 self-start rounded-md bg-muted p-0.5">
          <ViewTab active={active === "table"} onClick={() => setView("table")}>
            Keys
          </ViewTab>
          <ViewTab active={active === "code"} onClick={() => setView("code")}>
            File
          </ViewTab>
        </div>
      )}

      {active === "code" && hasCode ? (
        <CodeSide code={code} changed={changed} />
      ) : (
        <KeyedTable
          tree={tree}
          {...(locale ? { locale } : {})}
          {...(sourceLocale ? { sourceLocale } : {})}
          {...(onSelectBlock ? { onSelectBlock } : {})}
          {...(selectedBlockId ? { selectedBlockId } : {})}
          {...(blockAttrs ? { blockAttrs } : {})}
        />
      )}
    </div>
  );
}

function CodeSide({
  code,
  changed,
}: {
  code: DataCodeSource;
  changed: ReadonlySet<number>;
}): React.ReactElement {
  if (code.error) {
    return (
      <p className="p-6 text-center text-sm text-muted-foreground" role="status">
        {code.error}
      </p>
    );
  }
  if (code.text === undefined) {
    return (
      <p className="p-6 text-center text-sm text-muted-foreground" role="status">
        {code.loading ? "Writing the file…" : "The file is not loaded."}
      </p>
    );
  }
  return (
    <CodeView
      text={code.text}
      {...(code.filename ? { filename: code.filename } : {})}
      changedLines={changed}
      maxHeight="none"
      wrap
    />
  );
}

/** Re-exported so a host can build the table alone. */
export type { ContentTree, BlockAttrs };
