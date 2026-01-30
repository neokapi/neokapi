import type { SpanInfo } from "../../types/api";
import { TagChipComponent } from "./TagChipComponent";

interface TagPaletteProps {
  sourceSpans: SpanInfo[];
  onInsert: (spanInfo: SpanInfo) => void;
}

/**
 * Horizontal strip of source spans as clickable buttons for inserting into the target editor.
 * Each chip is numbered 1..N and can be inserted via click or Ctrl+1..9 keyboard shortcut.
 */
export function TagPalette({ sourceSpans, onInsert }: TagPaletteProps) {
  if (sourceSpans.length === 0) return null;

  return (
    <div style={paletteStyle}>
      <span style={labelStyle}>Tags:</span>
      {sourceSpans.map((span, i) => (
        <button
          key={i}
          onClick={() => onInsert(span)}
          style={buttonStyle}
          title={`Insert tag (Ctrl+${i + 1})`}
          data-testid={`tag-palette-${i}`}
        >
          <TagChipComponent spanInfo={span} index={i + 1} />
        </button>
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

const buttonStyle: React.CSSProperties = {
  background: "none",
  border: "none",
  padding: 0,
  cursor: "pointer",
  display: "inline-flex",
};
