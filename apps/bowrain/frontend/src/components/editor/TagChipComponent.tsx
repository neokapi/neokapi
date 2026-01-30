import type { SpanInfo } from "../../types/api";
import { spanLabel } from "./codedText";

interface TagChipComponentProps {
  spanInfo: SpanInfo;
  index?: number;
}

export function TagChipComponent({ spanInfo, index }: TagChipComponentProps) {
  const isPlaceholder = spanInfo.span_type === "placeholder";
  const label = spanLabel(spanInfo);

  return (
    <span
      style={{
        ...chipStyle,
        backgroundColor: isPlaceholder
          ? "rgba(251, 146, 60, 0.15)"
          : "rgba(96, 165, 250, 0.15)",
        borderColor: isPlaceholder
          ? "rgba(251, 146, 60, 0.4)"
          : "rgba(96, 165, 250, 0.4)",
        color: isPlaceholder
          ? "rgb(234, 88, 12)"
          : "rgb(59, 130, 246)",
      }}
      title={spanInfo.data}
      contentEditable={false}
      data-tag-chip
    >
      {index !== undefined && (
        <span style={indexStyle}>{index}</span>
      )}
      {label}
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
};

const indexStyle: React.CSSProperties = {
  fontSize: 9,
  fontWeight: 700,
  opacity: 0.6,
  marginRight: 1,
};
