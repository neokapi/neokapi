import { cn } from "@neokapi/ui-primitives";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useRef } from "react";
import type { BlockInfo } from "../../types/api";
import { FormattedSourceDisplay } from "./FormattedSourceDisplay";
import { HighlightedSource } from "./HighlightedSource";
import { UnifiedTargetEditor, type UnifiedSaveResult } from "../UnifiedTargetEditor";
import { CollapsedTargetCell } from "./GridTargetRenderer";
import { getBlockStatus, statusDotClass, statusBorderClass } from "./blockStatus";

export interface TableViewProps {
  blocks: BlockInfo[];
  targetLocale: string;
  targetLocaleLabel: string;
  selectedIndex: number;
  editingIndex: number | null;
  searchQuery: string;
  /** Term matches for the *currently selected* block (for inline highlight). */
  selectedTermMatches?: import("../../types/api").BlockTermMatch[];
  onSelect: (index: number) => void;
  onStartEditing: (index: number) => void;
  onCancelEditing: () => void;
  onSave: (index: number, result: UnifiedSaveResult) => void | Promise<void>;
}

/**
 * TableView is the Translate editor's row-based scanning surface — source /
 * target columns with a status accent, click-to-edit inline editing, and the
 * same UnifiedTargetEditor + chip rendering the Visual view uses. It is one of
 * the editor's two views (Visual being the other); it replaces the retired
 * grid / focus / split-h / split-v layout modes.
 */
export function TableView({
  blocks,
  targetLocale,
  targetLocaleLabel,
  selectedIndex,
  editingIndex,
  searchQuery,
  selectedTermMatches = [],
  onSelect,
  onStartEditing,
  onCancelEditing,
  onSave,
}: TableViewProps) {
  const listRef = useRef<HTMLDivElement>(null);

  // Virtualize the rows: a file can hold thousands of blocks, so only rows
  // near the viewport are mounted. Rows self-measure (source/target text and
  // the inline editor vary in height).
  const rowVirtualizer = useVirtualizer({
    count: blocks.length,
    getScrollElement: () => listRef.current,
    estimateSize: () => 56,
    overscan: 12,
    getItemKey: (index) => blocks[index].id,
    initialRect: { width: 800, height: 600 },
  });

  // Scroll selected block into view (the row may not be mounted yet).
  const { scrollToIndex } = rowVirtualizer;
  useEffect(() => {
    if (selectedIndex >= 0) scrollToIndex(selectedIndex, { align: "auto" });
  }, [selectedIndex, scrollToIndex]);

  return (
    <div
      ref={listRef}
      className="flex-1 overflow-auto border border-border rounded-lg bg-card"
      data-testid="block-grid"
    >
      {/* Header row */}
      <div className="flex px-3 py-2 text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider sticky top-0 bg-card backdrop-blur-sm z-[1]">
        <span className="w-10 text-center">#</span>
        <span className="w-4" />
        <span className="flex-1">Source</span>
        <span className="flex-1">Target ({targetLocaleLabel})</span>
      </div>

      {/* Block rows (virtualized — only rows near the viewport are mounted) */}
      <div
        style={{ height: `${rowVirtualizer.getTotalSize()}px`, width: "100%" }}
        className="relative"
      >
        {rowVirtualizer.getVirtualItems().map((virtualRow) => {
          const index = virtualRow.index;
          const block = blocks[index];
          const status = getBlockStatus(block, targetLocale);
          const isSelected = selectedIndex === index;
          return (
            <div
              key={block.id}
              ref={rowVirtualizer.measureElement}
              data-index={index}
              data-row-index={index}
              data-testid={`block-row-${index}`}
              onClick={() => {
                onSelect(index);
                if (editingIndex !== index) onCancelEditing();
              }}
              onDoubleClick={() => onStartEditing(index)}
              style={{ transform: `translateY(${virtualRow.start}px)` }}
              className={cn(
                "absolute top-0 left-0 w-full flex px-3 py-2 border-b border-border cursor-pointer items-stretch min-h-[44px] transition-colors border-l-[3px]",
                isSelected ? "bg-muted/50 border-l-primary" : statusBorderClass[status],
              )}
            >
              <span className="w-10 text-center text-xs text-muted-foreground pt-0.5 shrink-0">
                {index + 1}
              </span>
              <span
                className={cn("w-2 h-2 rounded-full shrink-0 mt-1.5 mr-2", statusDotClass[status])}
                data-testid={`status-dot-${index}`}
                title={status}
              />
              <div className="flex-1 text-sm leading-relaxed pr-4 break-words">
                {block.has_spans && block.source_coded && block.source_spans ? (
                  <FormattedSourceDisplay
                    codedText={block.source_coded}
                    spans={block.source_spans}
                  />
                ) : isSelected &&
                  (selectedTermMatches.length > 0 ||
                    (block.entities && block.entities.length > 0)) ? (
                  <HighlightedSource
                    text={block.source}
                    termMatches={selectedTermMatches}
                    entities={block.entities}
                  />
                ) : (
                  block.source
                )}
                {!block.translatable && (
                  <span className="ml-2 px-1.5 py-px bg-muted rounded text-[10px] text-muted-foreground align-middle">
                    non-translatable
                  </span>
                )}
              </div>
              <div
                className="flex-1 text-sm leading-relaxed break-words flex flex-col"
                data-testid={`target-cell-${index}`}
                onClick={(e) => {
                  if (editingIndex !== index) {
                    e.stopPropagation();
                    onSelect(index);
                    onStartEditing(index);
                  }
                }}
              >
                {editingIndex === index ? (
                  <UnifiedTargetEditor
                    block={block}
                    locale={targetLocale}
                    onSave={(result) => void onSave(index, result)}
                    onCancel={onCancelEditing}
                  />
                ) : (
                  <CollapsedTargetCell
                    block={block}
                    locale={targetLocale}
                    testId={`target-text-${index}`}
                  />
                )}
              </div>
            </div>
          );
        })}
      </div>

      {blocks.length === 0 && (
        <div className="p-6 text-center text-muted-foreground">
          {searchQuery ? "No blocks match the search query" : "No blocks found"}
        </div>
      )}
    </div>
  );
}
