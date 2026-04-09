// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { TMGroupedEntry } from "../components/resource-browser/TMGroupedEntry";
import type { TMGroupedResult } from "../components/resource-browser/types";

function makeGroup(overrides: Partial<TMGroupedResult> = {}): TMGroupedResult {
  return {
    source_text: "Hello world",
    source_coded: "Hello world",
    source_spans: [],
    source_locale: "en-US",
    targets: [
      {
        id: "t1",
        target_text: "Bonjour le monde",
        target_coded: "Bonjour le monde",
        target_spans: [],
        target_locale: "fr-FR",
        project_id: "",
        updated_at: new Date().toISOString(),
      },
      {
        id: "t2",
        target_text: "Hallo Welt",
        target_coded: "Hallo Welt",
        target_spans: [],
        target_locale: "de-DE",
        project_id: "",
        updated_at: new Date().toISOString(),
      },
    ],
    ...overrides,
  };
}

function renderToContainer(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

describe("TMGroupedEntry", () => {
  afterEach(() => {
    while (document.body.firstChild) {
      document.body.removeChild(document.body.firstChild);
    }
  });

  it("renders source text and translation count", () => {
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        group: makeGroup(),
        selected: false,
        onToggleSelect: vi.fn(),
        onEditTarget: vi.fn(),
        onDeleteTarget: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("Hello world");
    expect(c.textContent).toContain("2 translations");
  });

  it("renders source locale pill", () => {
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        group: makeGroup(),
        selected: false,
        onToggleSelect: vi.fn(),
        onEditTarget: vi.fn(),
        onDeleteTarget: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("en-US");
  });

  it("shows singular translation for single target", () => {
    const group = makeGroup({
      targets: [{
        id: "t1",
        target_text: "Bonjour",
        target_coded: "Bonjour",
        target_spans: [],
        target_locale: "fr-FR",
        project_id: "",
        updated_at: new Date().toISOString(),
      }],
    });
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        group,
        selected: false,
        onToggleSelect: vi.fn(),
        onEditTarget: vi.fn(),
        onDeleteTarget: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("1 translation");
  });

  it("auto-expands when fewer than 10 targets", () => {
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        group: makeGroup(),
        selected: false,
        onToggleSelect: vi.fn(),
        onEditTarget: vi.fn(),
        onDeleteTarget: vi.fn(),
      }),
    );
    // 2 targets < 10, so auto-expanded
    expect(c.textContent).toContain("Bonjour le monde");
    expect(c.textContent).toContain("Hallo Welt");
    expect(c.textContent).toContain("fr-FR");
    expect(c.textContent).toContain("de-DE");
  });

  it("collapses when clicking source on auto-expanded entry", () => {
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        group: makeGroup(),
        selected: false,
        onToggleSelect: vi.fn(),
        onEditTarget: vi.fn(),
        onDeleteTarget: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("Bonjour le monde");
    const buttons = c.querySelectorAll("button");
    const expandBtn = Array.from(buttons).find((b) => b.textContent?.includes("Hello world"));
    act(() => { expandBtn!.click(); });
    expect(c.textContent).not.toContain("Bonjour le monde");
  });

  it("filters targets by visibleLocales", () => {
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        group: makeGroup(),
        selected: false,
        onToggleSelect: vi.fn(),
        onEditTarget: vi.fn(),
        onDeleteTarget: vi.fn(),
        visibleLocales: ["fr-FR"],
      }),
    );
    expect(c.textContent).toContain("Bonjour le monde");
    expect(c.textContent).not.toContain("Hallo Welt");
    // Shows filtered count: 1/2
    expect(c.textContent).toContain("1/2");
  });

  it("calls onToggleSelect when checkbox clicked", () => {
    const onToggle = vi.fn();
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        group: makeGroup(),
        selected: false,
        onToggleSelect: onToggle,
        onEditTarget: vi.fn(),
        onDeleteTarget: vi.fn(),
      }),
    );
    const checkbox = c.querySelector("[data-slot=checkbox]") as HTMLElement;
    act(() => { checkbox.click(); });
    expect(onToggle).toHaveBeenCalledTimes(1);
  });
});
