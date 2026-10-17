import type { Meta, StoryObj } from "@storybook/react-vite";
import { useMemo, useState } from "react";
import { ATLAS_STYLE } from "../colors";
import { MapView, type MapMode } from "../components/MapView";
import {
  type AtlasQuery,
  type Pick,
  cohort,
  defaultQuery,
  evalContext,
  overlapping,
  prepareAtlas,
  previewSet,
} from "../model";
import type { AtlasData } from "../types";
import { makeAtlasFixture } from "./fixtures";

interface HarnessProps {
  data: AtlasData;
  initialFocus?: string;
  initialMode?: MapMode;
  /** A place selected as the map selects one: every market sharing ground with it is retained. */
  selected?: string;
}

/** The map with the query state ContextAtlas would hold around it. */
function MapHarness({ data, initialFocus, initialMode, selected }: HarnessProps) {
  const atlas = useMemo(() => prepareAtlas(data, { measure: "records" }), [data]);
  const [query, setQuery] = useState<AtlasQuery>(() => {
    const q = { ...defaultQuery(atlas), from: atlas.firstDay, view: "map" as const };
    if (!selected) return q;
    const markets = atlas.axisById.get("market")?.values.map((v) => v.id) ?? [];
    return { ...q, filters: { market: overlapping(atlas.places, selected, markets) } };
  });
  const [picks, setPicks] = useState<Pick[] | null>(null);
  const ctx = useMemo(() => evalContext(atlas, query), [atlas, query]);
  const foreground = useMemo(() => cohort(atlas, query, ctx), [atlas, query, ctx]);
  const preview = useMemo(
    () => (picks ? previewSet(atlas, foreground, picks, ctx) : null),
    [atlas, foreground, picks, ctx],
  );
  return (
    <div className="context-atlas" style={{ padding: 24 }}>
      <style>{ATLAS_STYLE}</style>
      <MapView
        atlas={atlas}
        query={query}
        ctx={ctx}
        foreground={foreground}
        preview={preview}
        onPreview={setPicks}
        onQuery={setQuery}
        initialFocus={initialFocus}
        initialMode={initialMode}
      />
    </div>
  );
}

const data = makeAtlasFixture();

const meta: Meta<typeof MapHarness> = {
  title: "Context atlas/Map",
  component: MapHarness,
  parameters: { layout: "fullscreen" },
  args: { data },
};
export default meta;

type Story = StoryObj<typeof MapHarness>;

/** The world by country. Records at the Nordics and Europe hatch the countries they cover. */
export const World: Story = { args: { initialFocus: "world" } };

/** Norway by county. Jan Mayen, which has no built-in shape, is listed beside the map. */
export const Norway: Story = { args: { initialFocus: "no" } };

/** Oslo county with the project's own places, one of them centred outside it. */
export const OsloCounty: Story = {
  args: { data: makeAtlasFixture({ misplaced: true }), initialFocus: "no-03" },
};

/** Norway with Greater Oslo selected: the records that hold there, the rest set aside. */
export const GreaterOsloSelected: Story = {
  args: { initialFocus: "no", selected: "greater-oslo" },
};

/** One equal tile per country. */
export const Grid: Story = { args: { initialMode: "grid" } };
