import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptView } from "../ConceptView";
import type { ConceptViewSlots } from "../ConceptView";
import type { ConceptDataSource } from "../adapter";
import { makeMemorySource } from "./fixtures";
import { demoSlots } from "./demo-panels";

const richSource = makeMemorySource();
const coreSource = makeMemorySource({ rich: false, editable: false });

/** Manages the centred concept so relation navigation re-centres the view. */
function ViewHarness({
  source,
  slots,
  start = "checkout",
}: {
  source: ConceptDataSource;
  slots?: ConceptViewSlots;
  start?: string;
}) {
  const [id, setId] = useState(start);
  return (
    <div className="mx-auto max-w-5xl p-6">
      <ConceptView
        conceptId={id}
        source={source}
        slots={slots}
        onNavigate={setId}
        onBack={fn()}
        onEdit={fn()}
      />
    </div>
  );
}

const meta: Meta<typeof ConceptView> = {
  title: "Concept UI/ConceptView",
  component: ConceptView,
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof ConceptView>;

/** The shell with no slots filled — header plus labelled placeholders. */
export const Scaffold: Story = {
  render: () => <ViewHarness source={richSource} />,
};

/**
 * The shell with the illustrative demo panels — geography, the local relations
 * widget (collapsing groups, navigation), the timeline, constraints, plus the
 * optional observations and comments.
 */
export const WithDemoPanels: Story = {
  render: () => <ViewHarness source={richSource} slots={demoSlots} />,
};

/**
 * A core-only source (local termbase): no markets/observations/comments/timeline
 * and no edit affordance. The demo panels degrade — geography is derived from
 * validity tags, the timeline is synthesised from timestamps.
 */
export const CoreOnly: Story = {
  render: () => (
    <ViewHarness
      source={coreSource}
      slots={{
        geography: demoSlots.geography,
        relations: demoSlots.relations,
        timeline: demoSlots.timeline,
        constraints: demoSlots.constraints,
      }}
    />
  ),
};
