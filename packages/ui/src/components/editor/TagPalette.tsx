import { useState, useMemo } from "react";
import type { SpanInfo } from "../../types/span";
import { TagChipComponent } from "./TagChipComponent";
import { buildPairs, semanticLabel, semanticCategory } from "./tagSemantics";
import { isCloneable } from "./tagConstraints";
import { cn } from "../../lib/utils";

interface TagPaletteProps {
  sourceSpans: SpanInfo[];
  onInsert: (spanInfo: SpanInfo) => void;
  usedSpans?: SpanInfo[];
  /** Show vocabulary category separators when spans cross categories. */
  showCategoryGroups?: boolean;
}

interface PairGroup {
  pairIndex: number;
  category: string;
  spans: { span: SpanInfo; sourceIndex: number }[];
}

const categoryShortLabels: Record<string, string> = {
  formatting: "Format",
  linking: "Links",
  media: "Media",
  structure: "Structure",
  code: "Code",
  generic: "Other",
};

/**
 * Horizontal strip of source spans as clickable buttons for inserting into the target editor.
 * Tags are grouped by pairs, with used tags dimmed and hover highlighting within pairs.
 * When spans cross multiple vocabulary categories, category labels are shown.
 */
export function TagPalette({
  sourceSpans,
  onInsert,
  usedSpans,
  showCategoryGroups,
}: TagPaletteProps) {
  if (sourceSpans.length === 0) return null;

  const pairs = useMemo(() => buildPairs(sourceSpans), [sourceSpans]);
  const [hoveredPairIndex, setHoveredPairIndex] = useState<number | null>(null);

  const groups = useMemo(() => {
    const groupMap = new Map<number, PairGroup>();
    const ordered: number[] = [];

    for (let i = 0; i < sourceSpans.length; i++) {
      const pairInfo = pairs.get(i);
      if (!pairInfo) continue;
      const { pairIndex } = pairInfo;

      if (!groupMap.has(pairIndex)) {
        groupMap.set(pairIndex, {
          pairIndex,
          category: semanticCategory(sourceSpans[i]),
          spans: [],
        });
        ordered.push(pairIndex);
      }
      groupMap.get(pairIndex)!.spans.push({ span: sourceSpans[i], sourceIndex: i });
    }

    return ordered.map((idx) => groupMap.get(idx)!);
  }, [sourceSpans, pairs]);

  const categories = useMemo(() => {
    const cats = new Set<string>();
    for (const g of groups) cats.add(g.category);
    return cats;
  }, [groups]);
  const multiCategory = showCategoryGroups !== false && categories.size > 1;

  const usedCounts = useMemo(() => {
    if (!usedSpans) return new Map<string, number>();
    const counts = new Map<string, number>();
    for (const span of usedSpans) {
      const key = `${span.type}:${span.span_type}`;
      counts.set(key, (counts.get(key) || 0) + 1);
    }
    return counts;
  }, [usedSpans]);

  const renderedCounts = new Map<string, number>();

  function isDimmed(span: SpanInfo): boolean {
    if (!usedSpans) return false;
    const key = `${span.type}:${span.span_type}`;
    const used = usedCounts.get(key) || 0;
    const rendered = renderedCounts.get(key) || 0;
    if (rendered < used) {
      renderedCounts.set(key, rendered + 1);
      return true;
    }
    return false;
  }

  let lastCategory = "";

  return (
    <div className="flex items-center gap-1 px-2 py-1 bg-[var(--bg-tertiary)] rounded mt-1 flex-wrap">
      <span className="text-[11px] text-[var(--text-secondary)] mr-1 font-medium">Tags:</span>
      {groups.map((group) => {
        const showSep = multiCategory && group.category !== lastCategory;
        lastCategory = group.category;

        return (
          <div key={group.pairIndex} className="contents">
            {showSep && (
              <span className="text-[9px] font-semibold text-[var(--text-secondary)] opacity-60 uppercase tracking-[0.05em] px-0.5">
                {categoryShortLabels[group.category] || group.category}
              </span>
            )}
            <div className="inline-flex items-center gap-0.5 px-1 py-px rounded-sm bg-black/[0.06] border border-black/10">
              <span className="text-[8px] font-bold text-[var(--text-secondary)] opacity-50 mr-0.5 min-w-[8px] text-center">
                {group.pairIndex}
              </span>
              {group.spans.map(({ span, sourceIndex }) => {
                const dimmed = isDimmed(span);
                const blocked = dimmed && !isCloneable(span);
                const label = semanticLabel(span);
                return (
                  <button
                    key={sourceIndex}
                    onClick={() => {
                      if (!blocked) onInsert(span);
                    }}
                    onMouseEnter={() => setHoveredPairIndex(group.pairIndex)}
                    onMouseLeave={() => setHoveredPairIndex(null)}
                    className={cn(
                      "bg-none border-none p-0 inline-flex",
                      blocked ? "cursor-not-allowed" : "cursor-pointer",
                    )}
                    title={
                      blocked
                        ? `"${label}" cannot be duplicated`
                        : `Insert tag (Ctrl+${sourceIndex + 1})`
                    }
                    disabled={blocked}
                    data-testid={`tag-palette-${sourceIndex}`}
                  >
                    <TagChipComponent
                      spanInfo={span}
                      index={sourceIndex + 1}
                      pairIndex={group.pairIndex}
                      highlighted={hoveredPairIndex === group.pairIndex}
                      dimmed={dimmed}
                      showConstraints
                    />
                  </button>
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
}
