import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  type CSSProperties,
  type ReactNode,
  type Ref,
} from "react";
import { useVirtualizer } from "@tanstack/react-virtual";

/** Alignment forwarded to the underlying virtualizer's scroll helpers. */
export type ScrollAlign = "auto" | "start" | "center" | "end";

/**
 * Props the primitive computes for each row's outer element. Spread them onto
 * the element your `renderRow` returns — they wire the measurement ref, the
 * absolute-position transform, and the `data-index` the virtualizer reads back
 * when it measures. In the non-virtualized fallback `style` is left undefined
 * and the ref is a no-op, so rows sit in normal document flow.
 */
export interface VirtualRowProps {
  ref?: (el: HTMLElement | null) => void;
  style?: CSSProperties;
  "data-index": number;
}

/** Arguments handed to the row renderer. */
export interface VirtualRowRenderArgs {
  index: number;
  /** Stable key derived from `getItemKey` (falls back to the index). */
  key: string | number;
  /** Spread onto the row's outer element (`<div {...rowProps}>…`). */
  rowProps: VirtualRowProps;
}

/** Imperative handle for scroll-follow that isn't driven by a prop. */
export interface VirtualListHandle {
  scrollToIndex: (index: number, opts?: { align?: ScrollAlign }) => void;
}

export interface VirtualListProps<T> {
  /** The full row set. Only the rows near the viewport are mounted. */
  items: T[];
  /** Estimated row height in px (used before a row is measured). */
  estimateSize: number;
  /** Rows to render beyond the visible window (smoother scroll). */
  overscan?: number;
  /** Stable per-row key (also the virtualizer's item key). */
  getItemKey?: (index: number, item: T) => string | number;
  /**
   * Seed rect so the virtualizer can produce rows before the viewport is
   * measured (first paint, and jsdom where a viewport reports zero height).
   */
  initialRect?: { width: number; height: number };
  /**
   * Scroll this index into view whenever it changes — for selection-follow.
   * Ignored when negative.
   */
  scrollToIndex?: number;
  /** Alignment for the `scrollToIndex` follow. */
  scrollAlign?: ScrollAlign;
  /**
   * Below this many items — and in environments that can't measure a viewport
   * (jsdom, a not-yet-laid-out container) — every row renders in normal flow,
   * so tests and small lists stay fully present in the DOM.
   */
  virtualizeThreshold?: number;
  /** Class for the scroll viewport. Give it a bounded height so the PAGE never
   *  becomes the scroll surface. */
  className?: string;
  /** data-slot for testing/inspection. */
  dataSlot?: string;
  /** data-testid on the scroll viewport. */
  dataTestId?: string;
  /** Sticky header rendered inside the scroll viewport, above the rows. */
  header?: ReactNode;
  /** Rendered when `items` is empty. */
  empty?: ReactNode;
  /** Render one row. Spread `args.rowProps` onto the outer element and add your
   *  own class/handlers/content. */
  renderRow: (item: T, args: VirtualRowRenderArgs) => ReactNode;
}

/**
 * VirtualList — the shared, data-source-agnostic virtualized list body that
 * every neokapi editor/review surface rebuilds across the Apache/AGPL boundary:
 * kapi-desktop's review queue and bowrain's Translate grid + Review list all
 * mount a `@tanstack/react-virtual` scroller with the same mechanics (bounded
 * scroll viewport, self-measuring absolute rows, an `initialRect`/threshold
 * fallback so jsdom and small lists render everything, and selection-follow
 * scrolling). This owns those mechanics once; the caller owns the row markup
 * via `renderRow`, keeping each surface's cells, handlers, and test hooks
 * exactly as they were (Apache-2.0 — imports only React + the MIT virtualizer).
 */
function VirtualListInner<T>(
  {
    items,
    estimateSize,
    overscan = 12,
    getItemKey,
    initialRect,
    scrollToIndex,
    scrollAlign = "auto",
    virtualizeThreshold = 80,
    className,
    dataSlot,
    dataTestId,
    header,
    empty,
    renderRow,
  }: VirtualListProps<T>,
  ref: Ref<VirtualListHandle>,
) {
  const parentRef = useRef<HTMLDivElement>(null);
  const wantVirtual = items.length > virtualizeThreshold;

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => estimateSize,
    overscan,
    enabled: wantVirtual,
    ...(getItemKey ? { getItemKey: (index: number) => getItemKey(index, items[index]) } : {}),
    ...(initialRect ? { initialRect } : {}),
  });

  const virtualItems = virtualizer.getVirtualItems();
  // The virtualizer reports zero rows in a viewport it can't measure (jsdom, or
  // a container that hasn't been laid out yet). Fall back to a plain render
  // there so nothing is silently dropped.
  const useVirtual = wantVirtual && virtualItems.length > 0;

  // Selection-follow: scroll the requested index into view. The row may not be
  // mounted yet, so route through the virtualizer.
  const { scrollToIndex: scrollIndex } = virtualizer;
  useEffect(() => {
    if (!useVirtual) return;
    if (scrollToIndex === undefined || scrollToIndex < 0) return;
    scrollIndex(scrollToIndex, { align: scrollAlign });
  }, [scrollToIndex, scrollAlign, useVirtual, scrollIndex]);

  useImperativeHandle(
    ref,
    () => ({
      scrollToIndex: (index: number, opts?: { align?: ScrollAlign }) =>
        virtualizer.scrollToIndex(index, { align: opts?.align ?? "auto" }),
    }),
    [virtualizer],
  );

  const keyFor = (index: number): string | number => getItemKey?.(index, items[index]) ?? index;

  return (
    <div
      ref={parentRef}
      className={className}
      data-slot={dataSlot}
      data-testid={dataTestId}
      style={{ overflowY: "auto" }}
    >
      {header}
      {items.length === 0 ? (
        empty
      ) : useVirtual ? (
        <div style={{ height: virtualizer.getTotalSize(), position: "relative", width: "100%" }}>
          {virtualItems.map((v) =>
            renderRow(items[v.index], {
              index: v.index,
              key: v.key as string | number,
              rowProps: {
                ref: virtualizer.measureElement,
                "data-index": v.index,
                style: {
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  transform: `translateY(${v.start}px)`,
                },
              },
            }),
          )}
        </div>
      ) : (
        items.map((item, i) =>
          renderRow(item, { index: i, key: keyFor(i), rowProps: { "data-index": i } }),
        )
      )}
    </div>
  );
}

/**
 * Generic-friendly wrapper: `forwardRef` erases the type parameter, so re-cast
 * to keep `VirtualList<T>` inference at call sites.
 */
export const VirtualList = forwardRef(VirtualListInner) as <T>(
  props: VirtualListProps<T> & { ref?: Ref<VirtualListHandle> },
) => ReactNode;
