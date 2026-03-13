import type { SpanInfo } from "../../types/api";
import { tagColors, semanticLabel, semanticTooltip, semanticCategory } from "./tagSemantics";
import { resolveConstraints } from "./tagConstraints";

interface TagChipComponentProps {
  spanInfo: SpanInfo;
  index?: number; // sequential number (1-based)
  pairIndex?: number; // pair number badge
  highlighted?: boolean; // partner-tag hover glow
  dimmed?: boolean; // already-used in palette
  locked?: boolean; // non-deletable tag (dashed border, lock tooltip)
  showConstraints?: boolean; // show constraint indicator icons
}

export function TagChipComponent({
  spanInfo,
  index,
  pairIndex,
  highlighted,
  dimmed,
  locked,
  showConstraints,
}: TagChipComponentProps) {
  const colors = tagColors(spanInfo);
  const label = semanticLabel(spanInfo);
  const constraints = resolveConstraints(spanInfo);
  const autoLocked = locked ?? !constraints.deletable;
  const tooltip = buildTooltip(spanInfo, constraints, autoLocked);

  return (
    <span
      style={{
        ...chipStyle,
        backgroundColor: colors.bg,
        borderColor: colors.border,
        borderStyle: autoLocked ? "dashed" : "solid",
        color: colors.text,
        boxShadow: highlighted ? `0 0 0 2px ${colors.border}` : undefined,
        opacity: dimmed ? 0.4 : undefined,
      }}
      title={tooltip}
      contentEditable={false}
      data-tag-chip
      data-span-type={spanInfo.type}
      data-category={semanticCategory(spanInfo)}
    >
      {index !== undefined && <span style={indexStyle}>{index}</span>}
      {label}
      {showConstraints && !constraints.deletable && (
        <span style={constraintIconStyle} aria-label="required">
          *
        </span>
      )}
      {pairIndex !== undefined && <span style={pairBadgeStyle}>{pairIndex}</span>}
    </span>
  );
}

/** Build a rich tooltip that includes semantic info, raw data, and constraint hints. */
function buildTooltip(
  span: SpanInfo,
  constraints: { deletable: boolean; cloneable: boolean; reorderable: boolean },
  locked: boolean,
): string {
  const base = semanticTooltip(span);
  const hints: string[] = [];
  if (locked || !constraints.deletable) hints.push("required in translation");
  if (!constraints.cloneable) hints.push("cannot be duplicated");
  if (!constraints.reorderable) hints.push("position is fixed");
  if (hints.length === 0) return base;
  return `${base}\n${hints.join(" · ")}`;
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

const constraintIconStyle: React.CSSProperties = {
  fontSize: 9,
  fontWeight: 700,
  opacity: 0.7,
  marginLeft: 1,
  lineHeight: 1,
};
