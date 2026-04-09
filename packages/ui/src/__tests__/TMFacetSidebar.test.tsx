// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { TMFacetSidebar, EMPTY_FACETS } from "../components/resource-browser/TMFacetSidebar";
import type { TMFacets } from "../components/resource-browser/types";

const FACETS: TMFacets = {
  locale_pairs: [
    { source_locale: "en-US", target_locale: "fr-FR", count: 10 },
    { source_locale: "en-US", target_locale: "de-DE", count: 5 },
  ],
  projects: [
    { project_id: "proj-1", count: 12 },
    { project_id: "", count: 6 },
  ],
  entity_types: [
    { type: "entity:person", count: 4 },
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
      createElement(TMFacetSidebar, { facets: FACETS, selection: EMPTY_FACETS, onSelectionChange: vi.fn() }),
    );
    expect(c.textContent).toContain("Filters");
  });

  it("renders target locale facets with counts", () => {
    const c = renderToContainer(
      createElement(TMFacetSidebar, { facets: FACETS, selection: EMPTY_FACETS, onSelectionChange: vi.fn() }),
    );
    expect(c.textContent).toContain("fr-FR");
    expect(c.textContent).toContain("de-DE");
    expect(c.textContent).toContain("10");
    expect(c.textContent).toContain("5");
  });

  it("calls onSelectionChange when locale checkbox is toggled", () => {
    const onChange = vi.fn();
    const c = renderToContainer(
      createElement(TMFacetSidebar, { facets: FACETS, selection: EMPTY_FACETS, onSelectionChange: onChange }),
    );
    // Find and click the first checkbox (fr-FR locale)
    const checkbox = c.querySelector("[data-slot=checkbox]") as HTMLElement;
    expect(checkbox).toBeTruthy();
    act(() => { checkbox.click(); });
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ targetLocales: ["fr-FR"] }),
    );
  });

  it("shows Clear all when filters are active", () => {
    const selection = { ...EMPTY_FACETS, targetLocales: ["fr-FR"] };
    const c = renderToContainer(
      createElement(TMFacetSidebar, { facets: FACETS, selection, onSelectionChange: vi.fn() }),
    );
    expect(c.textContent).toContain("Clear all");
  });

  it("does not show Clear all when no filters active", () => {
    const c = renderToContainer(
      createElement(TMFacetSidebar, { facets: FACETS, selection: EMPTY_FACETS, onSelectionChange: vi.fn() }),
    );
    expect(c.textContent).not.toContain("Clear all");
  });

  it("returns null when facets is null and not loading", () => {
    const c = renderToContainer(
      createElement(TMFacetSidebar, { facets: null, selection: EMPTY_FACETS, onSelectionChange: vi.fn() }),
    );
    expect(c.innerHTML).toBe("");
  });
});
