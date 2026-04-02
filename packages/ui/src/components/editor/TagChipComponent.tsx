import type { SpanInfo } from "../../types/span";
import { tagColors, semanticLabel, semanticTooltip, semanticCategory } from "./tagSemantics";
import { resolveConstraints } from "./tagConstraints";
import { cn } from "../../lib/utils";

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
      className={cn(
        "inline-flex items-center gap-0.5 px-1 mx-px rounded-sm border text-[11px] font-mono font-medium leading-[18px] align-middle cursor-default select-none whitespace-nowrap transition-[box-shadow,opacity] duration-150 ease-in-out",
        autoLocked ? "border-dashed" : "border-solid",
        dimmed && "opacity-40",
      )}
      style={{
        backgroundColor: colors.bg,
        borderColor: colors.border,
        color: colors.text,
        boxShadow: highlighted ? `0 0 0 2px ${colors.border}` : undefined,
      }}
      title={tooltip}
      contentEditable={false}
      data-tag-chip
      data-span-type={spanInfo.type}
      data-category={semanticCategory(spanInfo)}
    >
      {index !== undefined && <span className="text-[9px] font-bold opacity-60 mr-px">{index}</span>}
      {label}
      {showConstraints && !constraints.deletable && (
        <span className="text-[9px] font-bold opacity-70 ml-px leading-none" aria-label="required">
          *
        </span>
      )}
      {pairIndex !== undefined && <span className="text-[8px] font-bold opacity-50 ml-0.5 align-super leading-none">{pairIndex}</span>}
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
