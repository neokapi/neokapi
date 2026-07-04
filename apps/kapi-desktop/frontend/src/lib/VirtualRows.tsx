import { useRef, type ReactNode } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";

export interface VirtualRowsProps<T> {
  /** The full row set. Only the rows near the viewport are mounted. */
  items: T[];
  /** Estimated row height in px (used before measurement). */
  estimateSize: number;
  /** Render one row. `ref` MUST be spread onto the row's outer element so the
   *  virtualizer can measure its real height. */
  children: (item: T, index: number) => ReactNode;
  /** Rows to render beyond the visible window (smoother scroll). */
  overscan?: number;
  /** Class for the scroll viewport. Give it a bounded max-height so the PAGE
   *  never becomes the scroll surface. */
  className?: string;
  /** Optional sticky header rendered above the scrolling body. */
  header?: ReactNode;
  /** data-slot for testing/inspection. */
  dataSlot?: string;
  /** Below this many items, skip virtualization and render everything — keeps
   *  small lists (and jsdom tests, which report a 0-height viewport) honest and
   *  fully present in the DOM. */
  virtualizeThreshold?: number;
}

/**
 * A scroll-contained, optionally-virtualized vertical list. The viewport is a
 * bounded-height scroller (so the page never turns into a 10k-row scroll); when
 * the row count crosses `virtualizeThreshold` it mounts only the rows near the
 * viewport via @tanstack/react-virtual. Below the threshold — and in
 * environments that can't measure a viewport (jsdom) — every row renders, so
 * tests and small lists stay fully present.
 */
export function VirtualRows<T>({
  items,
  estimateSize,
  children,
  overscan = 12,
  className,
  header,
  dataSlot,
  virtualizeThreshold = 80,
}: VirtualRowsProps<T>) {
  const parentRef = useRef<HTMLDivElement>(null);
  const virtualize = items.length > virtualizeThreshold;

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => estimateSize,
    overscan,
    enabled: virtualize,
  });

  const virtualItems = virtualizer.getVirtualItems();
  // The virtualizer reports zero rows in a viewport it can't measure (jsdom, or
  // a not-yet-laid-out container). Fall back to a plain render there so nothing
  // is silently dropped.
  const useVirtual = virtualize && virtualItems.length > 0;

  return (
    <div className="flex min-h-0 flex-col">
      {header}
      <div ref={parentRef} className={className} data-slot={dataSlot} style={{ overflowY: "auto" }}>
        {useVirtual ? (
          <div style={{ height: virtualizer.getTotalSize(), position: "relative", width: "100%" }}>
            {virtualItems.map((v) => (
              <div
                key={v.key}
                data-index={v.index}
                ref={virtualizer.measureElement}
                style={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  transform: `translateY(${v.start}px)`,
                }}
              >
                {children(items[v.index], v.index)}
              </div>
            ))}
          </div>
        ) : (
          items.map((item, i) => children(item, i))
        )}
      </div>
    </div>
  );
}
