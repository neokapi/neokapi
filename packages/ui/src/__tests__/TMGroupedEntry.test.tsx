// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { TMGroupedEntry } from "../components/resource-browser/TMGroupedEntry";
import type { TMEntryDTO, VariantDTO } from "../components/resource-browser/types";

function v(locale: string, text: string): VariantDTO {
  return { locale, text, runs: [{ text }] };
}

function makeEntry(overrides: Partial<TMEntryDTO> = {}): TMEntryDTO {
  const now = new Date().toISOString();
  return {
    id: "tm-1",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": v("en-US", "Hello world"),
      "fr-FR": v("fr-FR", "Bonjour le monde"),
      "de-DE": v("de-DE", "Hallo Welt"),
    },
    created_at: now,
    updated_at: now,
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
        entry: makeEntry(),
        selected: false,
        onToggleSelect: vi.fn(),
        onEditVariant: vi.fn(),
        onDelete: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("Hello world");
    expect(c.textContent).toContain("2 translations");
  });

  it("renders source locale pill", () => {
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        entry: makeEntry(),
        selected: false,
        onToggleSelect: vi.fn(),
        onEditVariant: vi.fn(),
        onDelete: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("en-US");
  });

  it("shows singular translation for a single non-source variant", () => {
    const entry = makeEntry({
      variants: {
        "en-US": v("en-US", "Hello"),
        "fr-FR": v("fr-FR", "Bonjour"),
      },
    });
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        entry,
        selected: false,
        onToggleSelect: vi.fn(),
        onEditVariant: vi.fn(),
        onDelete: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("1 translation");
  });

  it("auto-expands when fewer than 10 non-source variants", () => {
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        entry: makeEntry(),
        selected: false,
        onToggleSelect: vi.fn(),
        onEditVariant: vi.fn(),
        onDelete: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("Bonjour le monde");
    expect(c.textContent).toContain("Hallo Welt");
    expect(c.textContent).toContain("fr-FR");
    expect(c.textContent).toContain("de-DE");
  });

  it("collapses when clicking source on auto-expanded entry", () => {
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        entry: makeEntry(),
        selected: false,
        onToggleSelect: vi.fn(),
        onEditVariant: vi.fn(),
        onDelete: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("Bonjour le monde");
    const buttons = c.querySelectorAll("button");
    const expandBtn = Array.from(buttons).find((b) => b.textContent?.includes("Hello world"));
    act(() => {
      expandBtn!.click();
    });
    expect(c.textContent).not.toContain("Bonjour le monde");
  });

  it("filters variants by visibleLocales", () => {
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        entry: makeEntry(),
        selected: false,
        onToggleSelect: vi.fn(),
        onEditVariant: vi.fn(),
        onDelete: vi.fn(),
        visibleLocales: ["fr-FR"],
      }),
    );
    expect(c.textContent).toContain("Bonjour le monde");
    expect(c.textContent).not.toContain("Hallo Welt");
    expect(c.textContent).toContain("1/2");
  });

  it("calls onToggleSelect when checkbox clicked", () => {
    const onToggle = vi.fn();
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        entry: makeEntry(),
        selected: false,
        onToggleSelect: onToggle,
        onEditVariant: vi.fn(),
        onDelete: vi.fn(),
      }),
    );
    const checkbox = c.querySelector("[data-slot=checkbox]") as HTMLElement;
    act(() => {
      checkbox.click();
    });
    expect(onToggle).toHaveBeenCalledTimes(1);
  });

  it("falls back to first locale when hint_src_lang is missing", () => {
    const entry = makeEntry({
      hint_src_lang: "",
      variants: {
        "fr-FR": v("fr-FR", "Bonjour"),
        "de-DE": v("de-DE", "Hallo"),
      },
    });
    const c = renderToContainer(
      createElement(TMGroupedEntry, {
        entry,
        selected: false,
        onToggleSelect: vi.fn(),
        onEditVariant: vi.fn(),
        onDelete: vi.fn(),
      }),
    );
    // The first variant ("fr-FR") is used as the header.
    expect(c.textContent).toContain("Bonjour");
    expect(c.textContent).toContain("Hallo");
  });
});
