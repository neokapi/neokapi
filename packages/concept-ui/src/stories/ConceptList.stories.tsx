import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptList } from "../ConceptList";
import { makeMemorySource } from "./fixtures";

const meta: Meta<typeof ConceptList> = {
  title: "Concept UI/ConceptList",
  component: ConceptList,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { onOpen: fn() },
  decorators: [
    (Story) => (
      <div className="mx-auto max-w-4xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConceptList>;

/** Full source: status, domain, source, and market filters all available. */
export const Default: Story = {
  args: { source: makeMemorySource() },
};

/** A core-only source (no named markets): the market filter is hidden. */
export const CoreOnly: Story = {
  args: { source: makeMemorySource({ rich: false, editable: false }) },
};

/** Opened on a starting filter. */
export const FilteredToPromotions: Story = {
  args: { source: makeMemorySource(), initialQuery: { domain: "promotions" } },
};
