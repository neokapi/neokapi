import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptDashboard } from "../ConceptDashboard";
import type { ConceptDataSource } from "../adapter";
import { makeMemorySource } from "./fixtures";

// The fully composed per-concept dashboard with the PRODUCTION section panels
// (geography, constraints, relations, timeline, observations, discussion) — the
// surface kapi-desktop and bowrain both render from just a data source.
const richSource = makeMemorySource();
const coreSource = makeMemorySource({ rich: false, editable: false });

function Harness({ source, start = "checkout" }: { source: ConceptDataSource; start?: string }) {
  const [id, setId] = useState(start);
  return (
    <div className="mx-auto max-w-5xl p-6">
      <ConceptDashboard
        conceptId={id}
        source={source}
        onNavigate={setId}
        onBack={fn()}
        onEdit={fn()}
      />
    </div>
  );
}

const meta = {
  title: "Concept UI/ConceptDashboard",
  component: ConceptDashboard,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ConceptDashboard>;
export default meta;
type Story = StoryObj<typeof meta>;

/** Platform path: rich data source (markets, observations, comments, timeline). */
export const Full: Story = {
  render: () => <Harness source={richSource} />,
};

/** Framework path: a local termbase only — core concept, terms, relations,
 *  tag-derived geography, constraints, and a synthesized timeline. Observations
 *  and discussion are absent and simply do not render. */
export const CoreOnly: Story = {
  render: () => <Harness source={coreSource} />,
};
