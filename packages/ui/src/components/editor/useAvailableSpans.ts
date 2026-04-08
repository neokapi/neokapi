import { useMemo } from "react";
import type { SpanInfo } from "../../types/span";
import { buildPairs, semanticLabel, semanticCategory } from "./tagSemantics";
import { isCloneable } from "./tagConstraints";

export interface AvailableSpanItem {
  span: SpanInfo;
  sourceIndex: number;
  pairIndex: number;
  category: string;
  label: string;
  /** Already used and not cloneable — cannot be inserted again. */
  blocked: boolean;
  /** Already present in target. */
  used: boolean;
}

export interface PairGroup {
  pairIndex: number;
  category: string;
  items: AvailableSpanItem[];
}

/**
 * Computes available source spans for insertion, grouped by pairs.
 * Shared between TagPalette, SelectionToolbarPlugin, and IntellisensePlugin.
 */
export function useAvailableSpans(
  sourceSpans: SpanInfo[],
  usedSpans?: SpanInfo[],
): { items: AvailableSpanItem[]; groups: PairGroup[] } {
  const pairs = useMemo(() => buildPairs(sourceSpans), [sourceSpans]);

  const usedCounts = useMemo(() => {
    if (!usedSpans) return new Map<string, number>();
    const counts = new Map<string, number>();
    for (const span of usedSpans) {
      const key = `${span.type}:${span.span_type}`;
      counts.set(key, (counts.get(key) || 0) + 1);
    }
    return counts;
  }, [usedSpans]);

  return useMemo(() => {
    const items: AvailableSpanItem[] = [];
    const groupMap = new Map<number, PairGroup>();
    const ordered: number[] = [];
    const renderedCounts = new Map<string, number>();

    for (let i = 0; i < sourceSpans.length; i++) {
      const pairInfo = pairs.get(i);
      if (!pairInfo) continue;
      const { pairIndex } = pairInfo;
      const span = sourceSpans[i];
      const key = `${span.type}:${span.span_type}`;

      const usedCount = usedCounts.get(key) || 0;
      const rendered = renderedCounts.get(key) || 0;
      const used = rendered < usedCount;
      if (used) renderedCounts.set(key, rendered + 1);

      const blocked = used && !isCloneable(span);

      const item: AvailableSpanItem = {
        span,
        sourceIndex: i,
        pairIndex,
        category: semanticCategory(span),
        label: semanticLabel(span),
        blocked,
        used,
      };
      items.push(item);

      if (!groupMap.has(pairIndex)) {
        groupMap.set(pairIndex, { pairIndex, category: semanticCategory(span), items: [] });
        ordered.push(pairIndex);
      }
      groupMap.get(pairIndex)!.items.push(item);
    }

    const groups = ordered.map((idx) => groupMap.get(idx)!);
    return { items, groups };
  }, [sourceSpans, pairs, usedCounts]);
}
