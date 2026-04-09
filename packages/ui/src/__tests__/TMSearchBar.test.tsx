// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { TMSearchBar } from "../components/resource-browser/TMSearchBar";

function renderToContainer(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

describe("TMSearchBar", () => {
  afterEach(() => {
    while (document.body.firstChild) {
      document.body.removeChild(document.body.firstChild);
    }
  });

  it("renders search input with placeholder", () => {
    const c = renderToContainer(
      createElement(TMSearchBar, {
        value: "",
        onChange: vi.fn(),
        sourceLocale: "en-US",
        targetLocale: "fr-FR",
        placeholder: "Search TM...",
      }),
    );
    const input = c.querySelector("input") as HTMLInputElement;
    expect(input).toBeTruthy();
    expect(input.placeholder).toBe("Search TM...");
  });

  it("renders with value", () => {
    const c = renderToContainer(
      createElement(TMSearchBar, {
        value: "hello",
        onChange: vi.fn(),
        sourceLocale: "en-US",
        targetLocale: "fr-FR",
      }),
    );
    const input = c.querySelector("input") as HTMLInputElement;
    expect(input.value).toBe("hello");
  });

  it("renders filter tokens", () => {
    const c = renderToContainer(
      createElement(TMSearchBar, {
        value: "",
        onChange: vi.fn(),
        filters: [{ key: "language", value: "fr-FR" }],
        onFiltersChange: vi.fn(),
        filterFields: [{ key: "language", label: "Language" }],
        sourceLocale: "en-US",
        targetLocale: "fr-FR",
      }),
    );
    expect(c.textContent).toContain("fr-FR");
  });

  it("does not show entity popover initially", () => {
    const c = renderToContainer(
      createElement(TMSearchBar, {
        value: "hello world",
        onChange: vi.fn(),
        onLookup: vi.fn().mockResolvedValue([]),
        sourceLocale: "en-US",
        targetLocale: "fr-FR",
      }),
    );
    expect(c.textContent).not.toContain("Mark");
  });

  it("invokes onEntitiesChange with empty array on mount", () => {
    const onEntitiesChange = vi.fn();
    renderToContainer(
      createElement(TMSearchBar, {
        value: "",
        onChange: vi.fn(),
        onEntitiesChange,
        sourceLocale: "en-US",
        targetLocale: "fr-FR",
      }),
    );
    expect(onEntitiesChange).toHaveBeenCalledWith([]);
  });

  it("does not show entity popover when onLookup is not provided, even with selection", () => {
    // Without onLookup, text selection should not trigger the popover.
    const c = renderToContainer(
      createElement(TMSearchBar, {
        value: "John works at Acme",
        onChange: vi.fn(),
        sourceLocale: "en-US",
        targetLocale: "fr-FR",
      }),
    );
    const input = c.querySelector("input") as HTMLInputElement;
    act(() => {
      input.setSelectionRange(0, 4);
      input.dispatchEvent(new Event("mouseup", { bubbles: true }));
    });
    // Popover should not be shown since onLookup is undefined
    expect(c.textContent).not.toContain("Mark");
  });
});
