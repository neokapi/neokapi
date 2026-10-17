import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ContextAtlas } from "../ContextAtlas";
import { CompareView } from "../components/CompareView";
import { ATLAS_STYLE } from "../colors";
import { DERIVED_AXES, applyQuestion, defaultQuery, prepareAtlas, withWindow } from "../model";
import { makeAtlasFixture, makeEmptyAtlas, makeOneRecordAtlas } from "./fixtures";

const data = makeAtlasFixture();
const atlas = prepareAtlas(data, { measure: "records" });
const base = defaultQuery(atlas);

const meta: Meta<typeof ContextAtlas> = {
  title: "Context atlas/ContextAtlas",
  component: ContextAtlas,
  parameters: { layout: "fullscreen" },
  args: { data, commandHint: "kapi context atlas" },
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof ContextAtlas>;

/** The atlas as it opens: the last 30 days, everything selected, the flow. */
export const Default: Story = {};

/** A project whose context holds nothing yet. */
export const EmptyData: Story = { args: { data: makeEmptyAtlas() } };

/** One rule, recorded and kept. */
export const OneRecord: Story = { args: { data: makeOneRecordAtlas() } };

/** Long labels and long evidence paths. */
export const LongLabels: Story = {
  args: {
    data: makeAtlasFixture({ longLabels: true }),
    initialQuery: { ...base, view: "records" },
  },
};

/** Two values retained on one axis and one on another. */
export const Retained: Story = {
  args: { initialQuery: { ...base, filters: { market: ["no", "se", "gb"], channel: ["app"] } } },
};

/** What needs a person: state at the window's start against the snapshot. */
export const NeedsAPerson: Story = {
  args: {
    initialQuery: applyQuestion(
      withWindow(base, "2026-08-25", "2026-09-25"),
      atlas,
      "needs-person",
    ),
  },
};

/** What changed in the window around the term change. */
export const WhatChanged: Story = {
  args: {
    initialQuery: applyQuestion(withWindow(base, "2026-08-28", "2026-09-10"), atlas, "changed"),
  },
};

/** Rules with a lifetime, during the summer campaign. */
export const RulesWithALifetime: Story = {
  args: {
    initialQuery: applyQuestion(withWindow(base, "2026-08-01", "2026-08-31"), atlas, "lifetime"),
  },
};

/** Two axes crossed. */
export const Matrix: Story = {
  args: { initialQuery: { ...base, view: "matrix", matrixRows: "product", matrixCols: "market" } },
};

/** The raster over the whole span. */
export const OverTime: Story = {
  args: {
    initialQuery: {
      ...withWindow(base, atlas.firstDay, atlas.lastDay),
      view: "time",
      rasterAxis: "product",
    },
  },
};

/** Markets on the map, from the world down to the project's own places. */
export const Map: Story = { args: { initialQuery: { ...base, view: "map" } } };

/** The records behind the counts. */
export const Records: Story = { args: { initialQuery: { ...base, view: "records" } } };

const contested =
  data.records.find(
    (r) => r.family === "rule" && r.history.some((h) => h.state === "contested") && r.valid_from,
  ) ?? data.records.find((r) => r.history.some((h) => h.state === "contested"));

/** A record open in the detail pane, among the contested ones. */
export const RecordOpen: Story = {
  args: {
    initialQuery: { ...base, filters: { [DERIVED_AXES.statusSnapshot]: ["contested"] } },
    initialRecord: contested?.id,
  },
};

function CompareStory() {
  const [open, setOpen] = useState(true);
  const a = withWindow(base, "2026-08-20", "2026-09-05");
  const b = {
    ...withWindow(base, "2026-09-05", "2026-09-25"),
    filters: { family: ["rule", "term"] },
  };
  return (
    <div className="context-atlas" style={{ padding: 24 }}>
      <style>{ATLAS_STYLE}</style>
      <button type="button" onClick={() => setOpen(true)}>
        Open
      </button>
      <CompareView
        atlas={atlas}
        pinned={a}
        query={b}
        open={open}
        onOpenChange={setOpen}
        onRestore={() => setOpen(false)}
        onSwap={() => undefined}
        onExport={() => undefined}
      />
    </div>
  );
}

/** A pinned against B: one category order, one scale. */
export const Compare: StoryObj = { render: () => <CompareStory /> };
