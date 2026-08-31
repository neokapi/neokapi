// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { EntryPlacement, formatPoint } from "../components/resource-browser/EntryPlacement";
import type {
  MemoryEntryDTO,
  MemoryPointDTO,
  VariantDTO,
} from "../components/resource-browser/types";

// KapiMart's shape: most entries carry the unit they were approved for and no
// point, because an answer every point agrees on is not a decision about a
// place. Two carry a point, because the same source was answered differently at
// two of them.

function v(locale: string, text: string): VariantDTO {
  return { locale, text, runs: [{ text }] };
}

function makeEntry(overrides: Partial<MemoryEntryDTO> = {}): MemoryEntryDTO {
  const now = new Date().toISOString();
  return {
    id: "tm-1",
    project_id: "kapimart",
    hint_src_lang: "en-US",
    variants: { "en-US": v("en-US", "Add to cart"), "nb-NO": v("nb-NO", "Legg i kurv") },
    created_at: now,
    updated_at: now,
    ...overrides,
  };
}

/** The corpus shape the browser renders: 45 unit-carrying, 2 with points. */
function makeCorpus(): MemoryEntryDTO[] {
  const out: MemoryEntryDTO[] = [];
  for (let i = 0; i < 45; i++) {
    out.push(makeEntry({ id: `tm-${i}`, unit: `web/en/index.md:b${i}` }));
  }
  out.push(
    makeEntry({
      id: "tm-c1",
      unit: "web/en/index.md:b0",
      point: { profile: "storefront", channel: "web" },
    }),
    makeEntry({
      id: "tm-c2",
      unit: "web/en/index.md:b0",
      point: { profile: "support", channel: "docs" },
    }),
  );
  return out;
}

async function renderToContainer(el: React.ReactElement): Promise<HTMLDivElement> {
  const container = document.createElement("div");
  document.body.appendChild(container);
  await act(async () => {
    createRoot(container).render(el);
  });
  return container;
}

afterEach(() => {
  while (document.body.firstChild) {
    document.body.removeChild(document.body.firstChild);
  }
});

describe("formatPoint", () => {
  it("reads the rungs coarsest first", () => {
    expect(formatPoint({ profile: "storefront", channel: "web" })).toBe("storefront/web");
    expect(formatPoint({ profile: "p", channel: "c", collection: "Docs" })).toBe("p/c/Docs");
    expect(formatPoint({ channel: "web" })).toBe("web");
  });

  it("has nothing to say about no point", () => {
    expect(formatPoint(undefined)).toBe("");
    expect(formatPoint(null)).toBe("");
    expect(formatPoint({})).toBe("");
  });
});

describe("EntryPlacement", () => {
  it("shows a contested entry's stored point without resolving one", async () => {
    const resolvePoint = vi.fn();
    const entry = makeCorpus()[45];
    const c = await renderToContainer(createElement(EntryPlacement, { entry, resolvePoint }));

    const point = c.querySelector("[data-testid=entry-point]");
    expect(point?.textContent).toBe("storefront/web");
    expect(point?.getAttribute("data-contested")).toBe("true");
    expect(resolvePoint).not.toHaveBeenCalled();
  });

  it("resolves the point of an uncontested entry through its unit", async () => {
    const resolvePoint = vi
      .fn<(e: MemoryEntryDTO) => Promise<MemoryPointDTO | null>>()
      .mockResolvedValue({ profile: "storefront", channel: "web" });
    const entry = makeEntry({ unit: "web/en/index.md:b7" });
    const c = await renderToContainer(createElement(EntryPlacement, { entry, resolvePoint }));

    expect(resolvePoint).toHaveBeenCalledWith(entry);
    const point = c.querySelector("[data-testid=entry-point]");
    expect(point?.textContent).toBe("storefront/web");
    expect(point?.getAttribute("data-contested")).toBe("false");
  });

  it("shows the unit, shortened to the part that identifies it", async () => {
    const c = await renderToContainer(
      createElement(EntryPlacement, { entry: makeEntry({ unit: "web/en/index.md:b7" }) }),
    );
    const unit = c.querySelector("[data-testid=entry-unit]");
    expect(unit?.textContent).toBe("b7");
  });

  it("draws no coordinate for a host that cannot resolve one", async () => {
    // Bowrain's adapter supplies no resolver, so nothing is guessed.
    const c = await renderToContainer(
      createElement(EntryPlacement, { entry: makeEntry({ unit: "web/en/index.md:b7" }) }),
    );
    expect(c.querySelector("[data-testid=entry-point]")).toBeNull();
    expect(c.querySelector("[data-testid=entry-unit]")).not.toBeNull();
  });

  it("renders nothing at all for an entry with neither", async () => {
    const c = await renderToContainer(createElement(EntryPlacement, { entry: makeEntry() }));
    expect(c.querySelector("[data-testid=entry-placement]")).toBeNull();
    expect(c.textContent).toBe("");
  });

  it("says nothing when the resolution fails", async () => {
    const resolvePoint = vi.fn().mockRejectedValue(new Error("no recipe"));
    const c = await renderToContainer(
      createElement(EntryPlacement, {
        entry: makeEntry({ unit: "web/en/index.md:b7" }),
        resolvePoint,
      }),
    );
    expect(c.querySelector("[data-testid=entry-point]")).toBeNull();
    // The entry is still a real answer; only its place is unknown.
    expect(c.querySelector("[data-testid=entry-unit]")).not.toBeNull();
  });

  it("opens the unit when a host offers somewhere to open it", async () => {
    const onOpenUnit = vi.fn();
    const entry = makeEntry({ unit: "web/en/index.md:b7" });
    const c = await renderToContainer(createElement(EntryPlacement, { entry, onOpenUnit }));

    const unit = c.querySelector<HTMLButtonElement>("button[data-testid=entry-unit]");
    expect(unit).not.toBeNull();
    await act(async () => {
      unit!.click();
    });
    expect(onOpenUnit).toHaveBeenCalledWith(entry);
  });

  it("renders the unit as a label where there is nowhere to open", async () => {
    const c = await renderToContainer(
      createElement(EntryPlacement, { entry: makeEntry({ unit: "web/en/index.md:b7" }) }),
    );
    expect(c.querySelector("button[data-testid=entry-unit]")).toBeNull();
    expect(c.querySelector("[data-testid=entry-unit]")?.textContent).toBe("b7");
  });

  it("resolves once per uncontested entry and never for a contested one", async () => {
    const corpus = makeCorpus();
    const resolvePoint = vi.fn().mockResolvedValue({ profile: "storefront", channel: "web" });

    for (const entry of corpus) {
      await renderToContainer(createElement(EntryPlacement, { entry, resolvePoint }));
    }
    // 45 carry a unit and no point; the 2 contested ones answer from their own
    // record rather than asking.
    expect(resolvePoint).toHaveBeenCalledTimes(45);
  });
});
