// @vitest-environment jsdom
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { VirtualList, type VirtualListHandle } from "../VirtualList";

interface Row {
  id: string;
  label: string;
}

const rows = (n: number): Row[] =>
  Array.from({ length: n }, (_, i) => ({ id: `r${i}`, label: `Row ${i}` }));

function renderList(props: Partial<React.ComponentProps<typeof VirtualList<Row>>> = {}) {
  const items = props.items ?? rows(5);
  return render(
    <VirtualList<Row>
      items={items}
      estimateSize={40}
      dataTestId="vlist"
      getItemKey={(_, it) => it.id}
      renderRow={(it, { rowProps, index }) => (
        <div key={it.id} {...rowProps} data-testid={`row-${index}`} data-id={it.id}>
          {it.label}
        </div>
      )}
      {...props}
    />,
  );
}

describe("VirtualList", () => {
  it("renders every row in the fallback (small list, below threshold)", () => {
    renderList({ items: rows(5) });
    // All five rows present and in normal flow (no absolute positioning).
    for (let i = 0; i < 5; i++) {
      expect(screen.getByTestId(`row-${i}`)).toBeInTheDocument();
    }
    expect(screen.getByTestId("row-0")).not.toHaveStyle({ position: "absolute" });
  });

  it("renders the sticky header and the scroll viewport slot", () => {
    render(
      <VirtualList<Row>
        items={rows(3)}
        estimateSize={40}
        dataSlot="my-list"
        header={<div data-testid="hdr">HEADER</div>}
        renderRow={(it, { rowProps }) => (
          <div key={it.id} {...rowProps}>
            {it.label}
          </div>
        )}
      />,
    );
    expect(screen.getByTestId("hdr")).toBeInTheDocument();
    expect(document.querySelector("[data-slot='my-list']")).not.toBeNull();
  });

  it("renders the empty slot when there are no items", () => {
    renderList({ items: [], empty: <div data-testid="empty">Nothing</div> });
    expect(screen.getByTestId("empty")).toBeInTheDocument();
    expect(screen.queryByTestId("row-0")).toBeNull();
  });

  it("never drops rows in jsdom, even past the threshold (the safety fallback)", () => {
    // jsdom has no layout engine, so the virtualizer measures every row as
    // zero-height and getVirtualItems() stays empty regardless of initialRect.
    // The fallback must then render the complete list rather than silently
    // dropping rows — true windowing is exercised in a real browser (Storybook).
    renderList({
      items: rows(500),
      virtualizeThreshold: 80,
      initialRect: { width: 800, height: 600 },
    });
    expect(document.querySelectorAll("[data-testid^='row-']").length).toBe(500);
  });

  it("still renders everything for a large list when the viewport can't be measured", () => {
    // No initialRect + jsdom's zero-height viewport → the virtualizer yields no
    // rows, so the fallback renders all of them rather than dropping any.
    renderList({ items: rows(120), virtualizeThreshold: 80 });
    expect(document.querySelectorAll("[data-testid^='row-']").length).toBe(120);
  });

  it("exposes an imperative scrollToIndex handle", () => {
    const ref = createRef<VirtualListHandle>();
    render(
      <VirtualList<Row>
        ref={ref}
        items={rows(200)}
        estimateSize={40}
        initialRect={{ width: 800, height: 600 }}
        getItemKey={(_, it) => it.id}
        renderRow={(it, { rowProps }) => (
          <div key={it.id} {...rowProps}>
            {it.label}
          </div>
        )}
      />,
    );
    expect(ref.current).not.toBeNull();
    // Should not throw; drives the virtualizer's scroll.
    expect(() => ref.current?.scrollToIndex(150, { align: "center" })).not.toThrow();
  });

  it("applies the declarative scrollToIndex follow without throwing", () => {
    const spy = vi.fn();
    // Re-render with a moving selection index; the effect runs on change.
    const { rerender } = render(
      <VirtualList<Row>
        items={rows(200)}
        estimateSize={40}
        initialRect={{ width: 800, height: 600 }}
        scrollToIndex={0}
        renderRow={(it, { rowProps }) => (
          <div key={it.id} {...rowProps} onClick={spy}>
            {it.label}
          </div>
        )}
      />,
    );
    expect(() =>
      rerender(
        <VirtualList<Row>
          items={rows(200)}
          estimateSize={40}
          initialRect={{ width: 800, height: 600 }}
          scrollToIndex={175}
          renderRow={(it, { rowProps }) => (
            <div key={it.id} {...rowProps} onClick={spy}>
              {it.label}
            </div>
          )}
        />,
      ),
    ).not.toThrow();
  });
});
