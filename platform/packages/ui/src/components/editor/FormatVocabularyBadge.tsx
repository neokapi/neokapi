import { useMemo } from "react";
import type { SpanInfo } from "../../types/api";
import { getDefaultRegistry } from "../../vocabularies";
import { cn } from "@neokapi/ui-primitives";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface FormatVocabularyBadgeProps {
  /** Spans present in the current block or file. */
  spans: SpanInfo[];
  /** Optional click handler to toggle the inline code legend. */
  onClick?: () => void;
}

// ---------------------------------------------------------------------------
// Category display info
// ---------------------------------------------------------------------------

const categoryIcons: Record<string, string> = {
  formatting: "Aa",
  linking: "Ln",
  media: "Img",
  structure: "St",
  code: "{ }",
};

const categoryOrder = ["formatting", "linking", "media", "structure", "code", "generic"];

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * Compact badge strip showing which vocabulary categories are active in the
 * current editing context. Clicking expands the InlineCodeLegend.
 */
export function FormatVocabularyBadge({ spans, onClick }: FormatVocabularyBadgeProps) {
  const registry = getDefaultRegistry();

  const summary = useMemo(() => {
    if (spans.length === 0) return null;

    const catCounts = new Map<string, number>();
    const seenTypes = new Set<string>();

    for (const span of spans) {
      const info = registry.lookupOrFallback(span.type);
      if (!seenTypes.has(span.type)) {
        seenTypes.add(span.type);
        catCounts.set(info.category, (catCounts.get(info.category) || 0) + 1);
      }
    }

    const entries = [...catCounts.entries()].sort(
      (a, b) => categoryOrder.indexOf(a[0]) - categoryOrder.indexOf(b[0]),
    );

    return {
      totalTypes: seenTypes.size,
      totalSpans: spans.length,
      categories: entries.map(([cat, count]) => ({
        category: cat,
        count,
        color:
          registry.typesInCategory(cat).length > 0
            ? registry.lookupOrFallback(registry.typesInCategory(cat)[0]).color
            : { bg: "transparent", border: "transparent", text: "var(--text-muted)" },
      })),
    };
  }, [spans, registry]);

  if (!summary) return null;

  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "inline-flex items-center gap-1.5 px-2 py-1 rounded-md text-[11px]",
        "bg-muted/40 hover:bg-muted/60 transition-colors border border-border/30",
        "cursor-pointer select-none",
      )}
      title={`${summary.totalSpans} inline tag${summary.totalSpans !== 1 ? "s" : ""} across ${summary.totalTypes} type${summary.totalTypes !== 1 ? "s" : ""}`}
    >
      {summary.categories.map(({ category, count, color }) => (
        <span
          key={category}
          className="inline-flex items-center gap-0.5"
          style={{ color: color.text }}
        >
          <span className="font-mono text-[10px] font-bold opacity-75">
            {categoryIcons[category] || "?"}
          </span>
          <span className="text-[9px] opacity-60">{count}</span>
        </span>
      ))}
      <span className="text-[10px] text-muted-foreground ml-0.5">
        {summary.totalSpans} tag{summary.totalSpans !== 1 ? "s" : ""}
      </span>
    </button>
  );
}
