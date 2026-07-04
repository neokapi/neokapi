// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { ListCapRow } from "../components/ui/list-cap-row";

function render(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

describe("ListCapRow", () => {
  it("renders nothing when the list was not truncated (total <= shown)", () => {
    const c = render(<ListCapRow shown={50} total={50} noun="files" />);
    expect(c.querySelector('[data-testid="list-cap-row"]')).toBeNull();
    const c2 = render(<ListCapRow shown={50} total={12} noun="files" />);
    expect(c2.querySelector('[data-testid="list-cap-row"]')).toBeNull();
  });

  it("renders an honest cap line when truncated, with thousands-formatted counts", () => {
    const c = render(<ListCapRow shown={500} total={12345} noun="files" />);
    const row = c.querySelector('[data-testid="list-cap-row"]');
    expect(row).not.toBeNull();
    expect(row?.textContent).toContain("Showing first 500 of 12,345 files");
  });

  it("uses the default noun and hint when not provided", () => {
    const c = render(<ListCapRow shown={10} total={99} />);
    const row = c.querySelector('[data-testid="list-cap-row"]');
    expect(row?.textContent).toContain("Showing first 10 of 99 items");
    expect(row?.textContent).toContain("Refine the filters to narrow the list.");
  });

  it("honours a custom hint", () => {
    const c = render(
      <ListCapRow shown={200} total={4000} noun="units" hint="Filter by status to see the rest." />,
    );
    const row = c.querySelector('[data-testid="list-cap-row"]');
    expect(row?.textContent).toContain("Filter by status to see the rest.");
  });
});
