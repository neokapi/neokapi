import { useState, useMemo } from "react";
import type { SpanInfo } from "../../types/api";
import { TagChipComponent } from "./TagChipComponent";
import { buildPairs } from "./tagSemantics";

interface TagPaletteProps {
  sourceSpans: SpanInfo[];
  onInsert: (spanInfo: SpanInfo) => void;
  usedSpans?: SpanInfo[];
}

interface PairGroup {
  pairIndex: number;
  spans: { span: SpanInfo; sourceIndex: number }[];
}

/**
 * Horizontal strip of source spans as clickable buttons for inserting into the target editor.
 * Tags are grouped by pairs, with used tags dimmed and hover highlighting within pairs.
 */
export function TagPalette({ sourceSpans, onInsert, usedSpans }: TagPaletteProps) {
  if (sourceSpans.length === 0) return null;

  const pairs = useMemo(() => buildPairs(sourceSpans), [sourceSpans]);
  const [hoveredPairIndex, setHoveredPairIndex] = useState<number | null>(null);

  // Build pair groups for display
  const groups = useMemo(() => {
    const groupMap = new Map<number, PairGroup>();
    const ordered: number[] = [];

    for (let i = 0; i < sourceSpans.length; i++) {
      const pairInfo = pairs.get(i);
      if (!pairInfo) continue;
      const { pairIndex } = pairInfo;

      if (!groupMap.has(pairIndex)) {
        groupMap.set(pairIndex, { pairIndex, spans: [] });
        ordered.push(pairIndex);
      }
      groupMap.get(pairIndex)!.spans.push({ span: sourceSpans[i], sourceIndex: i });
    }

    return ordered.map((idx) => groupMap.get(idx)!);
  }, [sourceSpans, pairs]);

  // Build used-tag counts by fingerprint for dimming
  const usedCounts = useMemo(() => {
    if (!usedSpans) return new Map<string, number>();
    const counts = new Map<string, number>();
    for (const span of usedSpans) {
      const key = `${span.type}:${span.span_type}`;
      counts.set(key, (counts.get(key) || 0) + 1);
    }
    return counts;
  }, [usedSpans]);

  // Track how many of each fingerprint have been rendered as dimmed
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

  return (
    <div style={paletteStyle}>
      <span style={labelStyle}>Tags:</span>
      {groups.map((group) => (
        <div key={group.pairIndex} style={groupContainerStyle}>
          <span style={pairLabelStyle}>{group.pairIndex}</span>
          {group.spans.map(({ span, sourceIndex }) => {
            const dimmed = isDimmed(span);
            return (
              <button
                key={sourceIndex}
                onClick={() => onInsert(span)}
                onMouseEnter={() => setHoveredPairIndex(group.pairIndex)}
                onMouseLeave={() => setHoveredPairIndex(null)}
                style={buttonStyle}
                title={`Insert tag (Ctrl+${sourceIndex + 1})`}
                data-testid={`tag-palette-${sourceIndex}`}
              >
                <TagChipComponent
                  spanInfo={span}
                  index={sourceIndex + 1}
                  pairIndex={group.pairIndex}
                  highlighted={hoveredPairIndex === group.pairIndex}
                  dimmed={dimmed}
                />
              </button>
            );
          })}
        </div>
      ))}
    </div>
  );
}

const paletteStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 4,
  padding: "4px 8px",
  backgroundColor: "var(--bg-tertiary)",
  borderRadius: 4,
  marginTop: 4,
  flexWrap: "wrap",
};

const labelStyle: React.CSSProperties = {
  fontSize: 11,
  color: "var(--text-secondary)",
  marginRight: 4,
  fontWeight: 500,
};

const groupContainerStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 2,
  padding: "1px 4px",
  borderRadius: 3,
  backgroundColor: "rgba(128, 128, 128, 0.06)",
  border: "1px solid rgba(128, 128, 128, 0.1)",
};

const pairLabelStyle: React.CSSProperties = {
  fontSize: 8,
  fontWeight: 700,
  color: "var(--text-secondary)",
  opacity: 0.5,
  marginRight: 2,
  minWidth: 8,
  textAlign: "center",
};

const buttonStyle: React.CSSProperties = {
  background: "none",
  border: "none",
  padding: 0,
  cursor: "pointer",
  display: "inline-flex",
};
