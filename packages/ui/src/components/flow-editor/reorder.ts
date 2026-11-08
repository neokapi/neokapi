// Drag-to-reorder for the linear flow editor's step list.
//
// Each row gets a grip button that starts a native HTML drag; hovering a row
// marks whether the step lands before or after it, and dropping moves the step
// there. The same grip moves the step with the arrow keys, so reordering works
// without a pointer.

import { useState, type DragEvent, type KeyboardEvent } from "react";

/** The drag type a step carries, so file drops and palette drags are ignored. */
const STEP_DRAG_TYPE = "application/x-neokapi-step";

export type DropSide = "before" | "after";

/** What a row needs to take part in reordering. */
export interface StepReorder {
  /** True while this row is the one being dragged. */
  dragging: boolean;
  /** Where the dragged step would land relative to this row, if here. */
  indicator?: DropSide;
  /** Spread on the grip button. */
  handle: {
    draggable: true;
    onDragStart: (e: DragEvent<HTMLElement>) => void;
    onDragEnd: () => void;
    onKeyDown: (e: KeyboardEvent<HTMLElement>) => void;
  };
  /** Spread on the row element. */
  row: {
    onDragOver: (e: DragEvent<HTMLElement>) => void;
    onDrop: (e: DragEvent<HTMLElement>) => void;
  };
}

/** The index a step dragged from `from` ends up at when dropped beside `index`. */
function landing(from: number, index: number, side: DropSide): number {
  const to = side === "before" ? index : index + 1;
  return from < to ? to - 1 : to;
}

/**
 * Reorder state for a list of `count` steps. `moveTo` receives the step's
 * current index and the index it should end up at.
 */
export function useStepReorder(count: number, moveTo: (from: number, to: number) => void) {
  const [dragged, setDragged] = useState<number | null>(null);
  const [over, setOver] = useState<{ index: number; side: DropSide } | null>(null);

  const reset = () => {
    setDragged(null);
    setOver(null);
  };

  return (index: number): StepReorder => ({
    dragging: dragged === index,
    indicator:
      dragged !== null && over?.index === index && landing(dragged, index, over.side) !== dragged
        ? over.side
        : undefined,
    handle: {
      draggable: true,
      onDragStart: (e) => {
        setDragged(index);
        e.dataTransfer.effectAllowed = "move";
        e.dataTransfer.setData(STEP_DRAG_TYPE, String(index));
        const row = e.currentTarget.closest("li");
        if (row) e.dataTransfer.setDragImage(row, 16, 16);
      },
      onDragEnd: reset,
      onKeyDown: (e) => {
        const to = e.key === "ArrowUp" ? index - 1 : e.key === "ArrowDown" ? index + 1 : null;
        if (to === null) return;
        e.preventDefault();
        if (to >= 0 && to < count) moveTo(index, to);
      },
    },
    row: {
      onDragOver: (e) => {
        if (dragged === null) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        const box = e.currentTarget.getBoundingClientRect();
        const side: DropSide = e.clientY < box.top + box.height / 2 ? "before" : "after";
        if (over?.index !== index || over.side !== side) setOver({ index, side });
      },
      onDrop: (e) => {
        if (dragged === null) return;
        e.preventDefault();
        const to = landing(dragged, index, over?.index === index ? over.side : "after");
        if (to !== dragged) moveTo(dragged, to);
        reset();
      },
    },
  });
}
