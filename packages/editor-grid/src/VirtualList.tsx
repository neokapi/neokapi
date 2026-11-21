import {
  useCallback,
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

/** Spread these props onto the row element so the list can measure and position it. */
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
  ref?: Ref<VirtualListHandle>;
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
   * Scroll this row into view when its index changes. A negative index clears selection.
   */
  scrollToIndex?: number;
  /** Alignment for the `scrollToIndex` follow. */
  scrollAlign?: ScrollAlign;
  /** Render every row until the list exceeds this size and its viewport can be measured. */
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

/** Render editor rows in a bounded viewport and keep the selected row visible. */
export function VirtualList<T>({
  ref,
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
}: VirtualListProps<T>) {
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

  const scrollToRow = useCallback(
    (index: number, opts?: { align?: ScrollAlign }) => {
      if (!Number.isInteger(index) || index < 0 || index >= items.length) return;
      const align = opts?.align ?? "auto";
      if (useVirtual) {
        virtualizer.scrollToIndex(index, { align });
      } else {
        parentRef.current?.querySelector<HTMLElement>(`[data-index="${index}"]`)?.scrollIntoView?.({
          block: align === "auto" ? "nearest" : align,
          inline: "nearest",
        });
      }
    },
    [items.length, useVirtual, virtualizer],
  );

  useEffect(() => {
    if (scrollToIndex !== undefined) scrollToRow(scrollToIndex, { align: scrollAlign });
  }, [scrollToIndex, scrollAlign, scrollToRow]);

  useImperativeHandle(ref, () => ({ scrollToIndex: scrollToRow }), [scrollToRow]);

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
