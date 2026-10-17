// @vitest-environment jsdom
import { useMemo, useState } from "react";
import { describe, expect, it, vi } from "vite-plus/test";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MapView, type MapMode } from "../components/MapView";
import {
  type AtlasQuery,
  type Pick,
  cohort,
  defaultQuery,
  evalContext,
  prepareAtlas,
  previewSet,
} from "../model";
import { makeAtlasFixture } from "../stories/fixtures";
import type { AtlasData } from "../types";

const data = makeAtlasFixture();

function Harness({
  atlasData = data,
  initialFocus,
  initialMode,
  onQuery,
}: {
  atlasData?: AtlasData;
  initialFocus?: string;
  initialMode?: MapMode;
  onQuery?: (q: AtlasQuery) => void;
}) {
  const atlas = useMemo(() => prepareAtlas(atlasData, { measure: "records" }), [atlasData]);
  const [query, setQuery] = useState<AtlasQuery>(() => ({
    ...defaultQuery(atlas),
    from: atlas.firstDay,
    view: "map",
  }));
  const [picks, setPicks] = useState<Pick[] | null>(null);
  const ctx = useMemo(() => evalContext(atlas, query), [atlas, query]);
  const foreground = useMemo(() => cohort(atlas, query, ctx), [atlas, query, ctx]);
  const preview = useMemo(
    () => (picks ? previewSet(atlas, foreground, picks, ctx) : null),
    [atlas, foreground, picks, ctx],
  );
  return (
    <MapView
      atlas={atlas}
      query={query}
      ctx={ctx}
      foreground={foreground}
      preview={preview}
      onPreview={setPicks}
      onQuery={(q) => {
        onQuery?.(q);
        setQuery(q);
      }}
      initialFocus={initialFocus}
      initialMode={initialMode}
    />
  );
}

const pane = () => screen.getByTestId("atlas-map-pane");
const crumbs = () => screen.getByRole("navigation", { name: "Place path" });

describe("MapView: shapes", () => {
  it("draws the world's countries from the built-in shapes, each a named button with its count", async () => {
    render(<Harness initialFocus="world" />);
    const no = await screen.findByTestId("atlas-map-place-no");
    expect(no.tagName.toLowerCase()).toBe("path");
    expect(no).toHaveAttribute("role", "button");
    expect(no).toHaveAttribute("tabindex", "0");
    expect(no.getAttribute("aria-label")).toMatch(/^Norway: \d+ selected/u);
    expect(no.getAttribute("d")).toMatch(/^M/u);
    expect(screen.getByTestId("atlas-map-shapes")).toHaveAttribute("aria-busy", "false");
    expect(within(crumbs()).getByText("World")).toHaveAttribute("aria-current", "page");
  });

  it("counts records at the world once, beside the map, rather than hatching the globe", async () => {
    render(<Harness initialFocus="world" />);
    await screen.findByTestId("atlas-map-place-no");
    const row = within(pane()).getByRole("button", { name: /Held everywhere/u });
    const atWorld = data.records.filter((r) => r.coordinates.market === "world").length;
    expect(atWorld).toBeGreaterThan(0);
    expect(row).toHaveTextContent(String(atWorld));
    // A country with nothing but world records is not hatched.
    const jp = screen.getByTestId("atlas-map-place-jp");
    expect(jp.parentElement?.querySelectorAll("path")).toHaveLength(1);
  });

  it("hatches a country where a record at a place made of others holds", async () => {
    render(<Harness initialFocus="world" />);
    const se = await screen.findByTestId("atlas-map-place-se");
    const layers = se.parentElement!.querySelectorAll("path");
    expect(layers).toHaveLength(2);
    expect(layers[1].getAttribute("fill")).toMatch(/^url\(#atlas-map-hatch-/u);
    expect(se.getAttribute("aria-label")).toMatch(/more hold here through .*Nordics/u);
  });

  it("drills from the world into a country, with a path back", async () => {
    const user = userEvent.setup();
    render(<Harness initialFocus="world" />);
    await screen.findByTestId("atlas-map-place-no");
    await user.click(within(pane()).getByRole("button", { name: "Show the places in Norway" }));
    expect(await screen.findByTestId("atlas-map-place-no-03")).toBeInTheDocument();
    expect(within(crumbs()).getByText("Norway")).toHaveAttribute("aria-current", "page");
    expect(
      within(crumbs())
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["World", "Europe", "Northern Europe"]);
    await user.click(within(crumbs()).getByRole("button", { name: "World" }));
    expect(await screen.findByTestId("atlas-map-place-se")).toBeInTheDocument();
    expect(screen.queryByTestId("atlas-map-place-no-03")).not.toBeInTheDocument();
  });

  it("draws every current Norwegian county, composed from the counties Natural Earth carries", async () => {
    render(<Harness initialFocus="no" />);
    await screen.findByTestId("atlas-map-place-no-03");
    for (const id of ["no-31", "no-32", "no-33", "no-34", "no-39", "no-40", "no-42", "no-46"]) {
      expect(screen.getByTestId(`atlas-map-place-${id}`).tagName.toLowerCase()).toBe("path");
    }
    for (const id of ["no-50", "no-55", "no-56", "no-11", "no-15", "no-18", "no-21"]) {
      expect(screen.getByTestId(`atlas-map-place-${id}`)).toBeInTheDocument();
    }
  });

  it("lists a subdivision with no built-in shape beside the map instead of inventing one", async () => {
    render(<Harness initialFocus="no" />);
    await screen.findByTestId("atlas-map-place-no-03");
    // Natural Earth carries no shape for Jan Mayen (no-22).
    expect(screen.queryByTestId("atlas-map-place-no-22")).not.toBeInTheDocument();
    expect(within(pane()).getByTestId("atlas-map-unplaced-no-22")).toBeInTheDocument();
    expect(
      within(pane()).getByRole("button", { name: /^Jan Mayen: .*listed, not drawn/u }),
    ).toBeInTheDocument();
  });

  it("lists a place made of others as a chip that outlines what it covers on hover", async () => {
    render(<Harness initialFocus="no" />);
    const oslo = await screen.findByTestId("atlas-map-place-no-03");
    const rogaland = screen.getByTestId("atlas-map-place-no-11");
    const chip = within(pane()).getByRole("button", { name: /^Greater Oslo/u });
    expect(oslo).toHaveAttribute("stroke-width", "0.6");
    fireEvent.mouseEnter(chip);
    expect(oslo).toHaveAttribute("stroke", "var(--foreground)");
    expect(rogaland).not.toHaveAttribute("stroke", "var(--foreground)");
    fireEvent.mouseLeave(chip);
    expect(oslo).toHaveAttribute("stroke-width", "0.6");
  });

  it("draws a declared place's own shape, and one without a shape at its centre", async () => {
    render(<Harness initialFocus="no-46" />);
    const bergen = await screen.findByTestId("atlas-map-place-bergen");
    expect(bergen.tagName.toLowerCase()).toBe("path");
    render(<Harness initialFocus="no-03" />);
    const oslo = await screen.findByTestId("atlas-map-place-oslo");
    expect(oslo.tagName.toLowerCase()).toBe("circle");
  });

  it("lists a declared place with neither shape nor centre", async () => {
    render(<Harness initialFocus="oslo" />);
    await screen.findByTestId("atlas-map-place-grunerlokka");
    expect(screen.queryByTestId("atlas-map-place-frogner")).not.toBeInTheDocument();
    expect(within(pane()).getByTestId("atlas-map-unplaced-frogner")).toBeInTheDocument();
  });

  it("flags a declared place whose centre falls outside the place it sits within", async () => {
    render(<Harness atlasData={makeAtlasFixture({ misplaced: true })} initialFocus="no-03" />);
    await screen.findByTestId("atlas-map-place-oslo");
    expect(within(pane()).getByTestId("atlas-map-flag-holmenkollen")).toBeInTheDocument();
    expect(within(pane()).queryByTestId("atlas-map-flag-oslo")).not.toBeInTheDocument();
    expect(
      within(pane()).getByRole("button", { name: /^Holmenkollen: .*centre lies outside Oslo/u }),
    ).toBeInTheDocument();
  });

  it("retains every record at a place sharing ground with the one selected", async () => {
    const onQuery = vi.fn();
    render(<Harness initialFocus="no" onQuery={onQuery} />);
    const county = await screen.findByTestId("atlas-map-place-no-03");
    fireEvent.click(county);
    const kept = onQuery.mock.lastCall![0].filters.market as string[];
    expect(kept).toEqual(
      expect.arrayContaining(["no", "oslo", "grunerlokka", "greater-oslo", "nordics", "world"]),
    );
    expect(kept).not.toContain("bergen");
    expect(kept).not.toContain("se");
    expect(county).toHaveAttribute("aria-pressed", "true");
  });

  it("marks as selected only the places a selection keeps records within", async () => {
    const onQuery = vi.fn();
    render(<Harness initialFocus="no" onQuery={onQuery} />);
    const county = await screen.findByTestId("atlas-map-place-no-03");
    fireEvent.click(within(pane()).getByRole("button", { name: /^Greater Oslo/u }));
    expect(onQuery.mock.lastCall![0].filters.market).toEqual(
      expect.arrayContaining(["greater-oslo", "oslo", "no", "nordics"]),
    );
    expect(county).toHaveAttribute("aria-pressed", "true");
    // Rogaland holds the Norway records the selection keeps, but none sits within it.
    expect(screen.getByTestId("atlas-map-place-no-11")).toHaveAttribute("aria-pressed", "false");
  });

  it("selects with Enter on a focused place", async () => {
    const user = userEvent.setup();
    const onQuery = vi.fn();
    render(<Harness initialFocus="world" onQuery={onQuery} />);
    const de = await screen.findByTestId("atlas-map-place-de");
    de.focus();
    await user.keyboard("{Enter}");
    expect(onQuery.mock.lastCall![0].filters.market).toEqual(
      expect.arrayContaining(["de", "europe", "world"]),
    );
  });

  it("opens at the deepest country or region holding every record", async () => {
    const oslo = makeAtlasFixture();
    oslo.records = oslo.records.filter((r) =>
      ["oslo", "bergen", "no-03"].includes(r.coordinates.market ?? ""),
    );
    render(<Harness atlasData={oslo} />);
    await screen.findByTestId("atlas-map-place-no-03");
    expect(within(crumbs()).getByText("Norway")).toHaveAttribute("aria-current", "page");
  });
});

describe("MapView: grid", () => {
  it("switches to one equal tile per country where records sit or hold", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Grid" }));
    const grid = screen.getByTestId("atlas-map-grid");
    expect(within(grid).getByTestId("atlas-map-tile-no")).toHaveAttribute("role", "button");
    // Portugal holds a Europe record without one of its own.
    expect(within(grid).getByTestId("atlas-map-tile-pt")).toBeInTheDocument();
    // No record sits or holds in Brazil.
    expect(within(grid).queryByTestId("atlas-map-tile-br")).not.toBeInTheDocument();
  });

  it("opens in grid mode when asked", () => {
    render(<Harness initialMode="grid" />);
    expect(screen.getByTestId("atlas-map-grid")).toBeInTheDocument();
  });
});
