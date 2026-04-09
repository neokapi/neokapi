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

  it("renders actions slot", () => {
    const c = renderToContainer(
      createElement(TMSearchBar, {
        value: "",
        onChange: vi.fn(),
        sourceLocale: "en-US",
        targetLocale: "fr-FR",
        actions: createElement("button", { "data-testid": "custom-action" }, "Action"),
      }),
    );
    expect(c.querySelector("[data-testid=custom-action]")).toBeTruthy();
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
});
