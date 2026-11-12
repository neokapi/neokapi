/** A rectangle in viewport coordinates. */
export interface Box {
  top: number;
  left: number;
  width: number;
  height: number;
}

export type Placement = "top" | "bottom" | "left" | "right";

export interface BubblePosition {
  top: number;
  left: number;
  /** The side of the target the bubble sits on, or `center` when there is no target. */
  placement: Placement | "center";
}

/** Space between the cutout and the bubble, in px. */
export const BUBBLE_GAP = 12;
/** Space the bubble keeps from the window edges, in px. */
export const VIEWPORT_MARGIN = 8;
/** Space between the target and the edge of its cutout, in px. */
export const CUTOUT_PADDING = 6;

const OPPOSITE: Record<Placement, Placement> = {
  top: "bottom",
  bottom: "top",
  left: "right",
  right: "left",
};

function clamp(value: number, min: number, max: number): number {
  if (max < min) return min;
  return Math.min(Math.max(value, min), max);
}

/** The cutout around a target: its rect grown by the padding on every side. */
export function cutoutBox(rect: Box, padding = CUTOUT_PADDING): Box {
  return {
    top: rect.top - padding,
    left: rect.left - padding,
    width: rect.width + padding * 2,
    height: rect.height + padding * 2,
  };
}

function fits(side: Placement, target: Box, bubble: Size, viewport: Size): boolean {
  const m = VIEWPORT_MARGIN;
  switch (side) {
    case "top":
      return target.top - BUBBLE_GAP - bubble.height >= m;
    case "bottom":
      return target.top + target.height + BUBBLE_GAP + bubble.height <= viewport.height - m;
    case "left":
      return target.left - BUBBLE_GAP - bubble.width >= m;
    case "right":
      return target.left + target.width + BUBBLE_GAP + bubble.width <= viewport.width - m;
  }
}

export interface Size {
  width: number;
  height: number;
}

/**
 * Where the bubble goes. It takes the preferred side when the bubble fits
 * there, then the opposite side, then the two remaining sides, and falls back
 * to the preferred side when none fits. The cross axis centres the bubble on
 * the target. Both axes are clamped so the bubble stays inside the window.
 * With no target the bubble sits in the middle of the window.
 */
export function placeBubble(
  target: Box | null,
  bubble: Size,
  viewport: Size,
  preferred: Placement = "bottom",
): BubblePosition {
  const m = VIEWPORT_MARGIN;
  const maxTop = viewport.height - bubble.height - m;
  const maxLeft = viewport.width - bubble.width - m;
  if (!target) {
    return {
      top: clamp((viewport.height - bubble.height) / 2, m, maxTop),
      left: clamp((viewport.width - bubble.width) / 2, m, maxLeft),
      placement: "center",
    };
  }
  const order: Placement[] = [preferred, OPPOSITE[preferred]];
  for (const side of ["bottom", "top", "right", "left"] as Placement[]) {
    if (!order.includes(side)) order.push(side);
  }
  const side = order.find((s) => fits(s, target, bubble, viewport)) ?? preferred;

  let top: number;
  let left: number;
  if (side === "top" || side === "bottom") {
    top =
      side === "top"
        ? target.top - BUBBLE_GAP - bubble.height
        : target.top + target.height + BUBBLE_GAP;
    left = target.left + target.width / 2 - bubble.width / 2;
  } else {
    left =
      side === "left"
        ? target.left - BUBBLE_GAP - bubble.width
        : target.left + target.width + BUBBLE_GAP;
    top = target.top + target.height / 2 - bubble.height / 2;
  }
  return { top: clamp(top, m, maxTop), left: clamp(left, m, maxLeft), placement: side };
}

/** Whether a rect lies wholly inside the window. */
export function isInViewport(rect: Box, viewport: Size): boolean {
  return (
    rect.top >= 0 &&
    rect.left >= 0 &&
    rect.top + rect.height <= viewport.height &&
    rect.left + rect.width <= viewport.width
  );
}
