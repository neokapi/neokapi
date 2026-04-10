// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { TMFacetSidebar, EMPTY_FACETS } from "../components/resource-browser/TMFacetSidebar";
import type { TMFacets } from "../components/resource-browser/types";

const FACETS: TMFacets = {
  locales: [
    { locale: "en-US", count: 15 },
    { locale: "fr-FR", count: 10 },
    { locale: "de-DE", count: 5 },
  ],
  projects: [
    { project_id: "proj-1", count: 12 },
    { project_id: "", count: 6 },
  ],
  entity_types: [{ type: "entity:person", count: 4 }],
  import_sessions: [
    {
      session_id: "sess-1",
      file_key: "acme-glossary.tmx",
      tool_name: "tmx-import",
      imported_at: new Date().toISOString(),
      count: 12,
    },
    {
      session_id: "sess-2",
      file_key: "legacy.tmx",
      imported_at: new Date().toISOString(),
      count: 4,
    },
  ],
  has_codes: 8,
  no_codes: 10,
};

function renderToContainer(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

describe("TMFacetSidebar", () => {
  afterEach(() => {
    while (document.body.firstChild) {
      document.body.removeChild(document.body.firstChild);
    }
  });

  it("renders Filters heading", () => {
    const c = renderToContainer(
      createElement(TMFacetSidebar, {
        facets: FACETS,
        selection: EMPTY_FACETS,
        onSelectionChange: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("Filters");
  });

  it("renders Languages facets with counts", () => {
    const c = renderToContainer(
      createElement(TMFacetSidebar, {
        facets: FACETS,
        selection: EMPTY_FACETS,
        onSelectionChange: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("Languages");
    expect(c.textContent).toContain("en-US");
    expect(c.textContent).toContain("fr-FR");
    expect(c.textContent).toContain("de-DE");
    expect(c.textContent).toContain("15");
    expect(c.textContent).toContain("10");
    expect(c.textContent).toContain("5");
  });

  it("renders Import Sessions facets", () => {
    const c = renderToContainer(
      createElement(TMFacetSidebar, {
        facets: FACETS,
        selection: EMPTY_FACETS,
        onSelectionChange: vi.fn(),
      }),
    );
    expect(c.textContent).toContain("Import Sessions");
    // Expand the section before reading items.
    const trigger = Array.from(c.querySelectorAll("button")).find((b) =>
      b.textContent?.includes("Import Sessions"),
    );
    expect(trigger).toBeTruthy();
    act(() => {
      trigger!.click();
    });
    expect(c.textContent).toContain("acme-glossary.tmx");
    expect(c.textContent).toContain("legacy.tmx");
  });

  it("calls onSelectionChange when locale checkbox is toggled", () => {
    const onChange = vi.fn();
    const c = renderToContainer(
      createElement(TMFacetSidebar, {
        facets: FACETS,
        selection: EMPTY_FACETS,
        onSelectionChange: onChange,
      }),
    );
    const checkbox = c.querySelector("[data-slot=checkbox]") as HTMLElement;
    expect(checkbox).toBeTruthy();
    act(() => {
      checkbox.click();
    });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ locales: ["en-US"] }));
  });

  it("calls onSelectionChange when a session is toggled", () => {
    const onChange = vi.fn();
    const c = renderToContainer(
      createElement(TMFacetSidebar, {
        facets: FACETS,
        selection: EMPTY_FACETS,
        onSelectionChange: onChange,
      }),
    );
    // Expand the Import Sessions section first.
    const trigger = Array.from(c.querySelectorAll("button")).find((b) =>
      b.textContent?.includes("Import Sessions"),
    );
    act(() => {
      trigger!.click();
    });
    const labels = Array.from(c.querySelectorAll("label"));
    const sessionLabel = labels.find((l) => l.textContent?.includes("acme-glossary.tmx"));
    const sessionBox = sessionLabel?.querySelector("[data-slot=checkbox]") as HTMLElement | null;
    expect(sessionBox).toBeTruthy();
    act(() => {
      sessionBox!.click();
    });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ sessionIds: ["sess-1"] }));
  });

  it("shows Clear all when filters are active", () => {
    const selection = { ...EMPTY_FACETS, locales: ["fr-FR"] };
    const c = renderToContainer(
      createElement(TMFacetSidebar, { facets: FACETS, selection, onSelectionChange: vi.fn() }),
    );
    expect(c.textContent).toContain("Clear all");
  });

  it("does not show Clear all when no filters active", () => {
    const c = renderToContainer(
      createElement(TMFacetSidebar, {
        facets: FACETS,
        selection: EMPTY_FACETS,
        onSelectionChange: vi.fn(),
      }),
    );
    expect(c.textContent).not.toContain("Clear all");
  });

  it("returns null when facets is null and not loading", () => {
    const c = renderToContainer(
      createElement(TMFacetSidebar, {
        facets: null,
        selection: EMPTY_FACETS,
        onSelectionChange: vi.fn(),
      }),
    );
    expect(c.innerHTML).toBe("");
  });
});
