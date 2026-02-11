import type { SpanInfo } from "../../types/api";
import { tagColors, semanticLabel, semanticTooltip } from "./tagSemantics";

interface TagChipComponentProps {
  spanInfo: SpanInfo;
  index?: number;         // sequential number (1-based)
  pairIndex?: number;     // pair number badge
  highlighted?: boolean;  // partner-tag hover glow
  dimmed?: boolean;       // already-used in palette
}

export function TagChipComponent({ spanInfo, index, pairIndex, highlighted, dimmed }: TagChipComponentProps) {
  const colors = tagColors(spanInfo);
  const label = semanticLabel(spanInfo);
  const tooltip = semanticTooltip(spanInfo);

  return (
    <span
      style={{
        ...chipStyle,
        backgroundColor: colors.bg,
        borderColor: colors.border,
        color: colors.text,
        boxShadow: highlighted ? `0 0 0 2px ${colors.border}` : undefined,
        opacity: dimmed ? 0.4 : undefined,
      }}
      title={tooltip}
      contentEditable={false}
      data-tag-chip
    >
      {index !== undefined && (
        <span style={indexStyle}>{index}</span>
      )}
      {label}
      {pairIndex !== undefined && (
        <span style={pairBadgeStyle}>{pairIndex}</span>
      )}
    </span>
  );
}

const chipStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 2,
  padding: "0 4px",
  margin: "0 1px",
  borderRadius: 3,
  border: "1px solid",
  fontSize: 11,
  fontFamily: "monospace",
  fontWeight: 500,
  lineHeight: "18px",
  verticalAlign: "middle",
  cursor: "default",
  userSelect: "none",
  whiteSpace: "nowrap",
  transition: "box-shadow 0.15s ease, opacity 0.15s ease",
};

const indexStyle: React.CSSProperties = {
  fontSize: 9,
  fontWeight: 700,
  opacity: 0.6,
  marginRight: 1,
};

const pairBadgeStyle: React.CSSProperties = {
  fontSize: 8,
  fontWeight: 700,
  opacity: 0.5,
  marginLeft: 2,
  verticalAlign: "super",
  lineHeight: 1,
};
