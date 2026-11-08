// The grip that drags a step to a new place in the flow, or moves it with the
// arrow keys when focused.

import { GripVertical } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "../../lib/utils";
import { SimpleTooltip } from "../ui/tooltip";
import type { DropSide, StepReorder } from "./reorder";

export function ReorderHandle({
  reorder,
  index,
  name,
}: {
  reorder: StepReorder;
  index: number;
  name: string;
}) {
  return (
    <SimpleTooltip content={t("Drag to reorder, or use the arrow keys")}>
      <button
        type="button"
        {...reorder.handle}
        data-reorder-handle={index}
        aria-label={t("Reorder {step}", { step: name })}
        className="mt-0.5 cursor-grab rounded text-muted-foreground/60 hover:text-foreground focus-visible:text-foreground outline-none focus-visible:ring-3 focus-visible:ring-ring/50 active:cursor-grabbing"
      >
        <GripVertical className="size-4" />
      </button>
    </SimpleTooltip>
  );
}

/** The line drawn in the gap where a dragged step would land. */
export function DropIndicator({ side }: { side?: DropSide }) {
  if (!side) return null;
  return (
    <span
      aria-hidden="true"
      data-slot="drop-indicator"
      data-side={side}
      className={cn(
        "pointer-events-none absolute inset-x-0 h-0.5 rounded-full bg-primary",
        side === "before" ? "-top-[5px]" : "-bottom-[5px]",
      )}
    />
  );
}
