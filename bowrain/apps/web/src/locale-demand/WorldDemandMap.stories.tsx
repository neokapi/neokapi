import type { Meta, StoryObj } from "@storybook/react-vite";
import { WorldDemandMap } from "./WorldDemandMap";
import { sampleDemandDataSource } from "./locale-demand-fixtures";

// Locale demand — DESIGN PROTOTYPE (mock data only).

const snapshot = sampleDemandDataSource.getSnapshot("30d");

const meta: Meta<typeof WorldDemandMap> = {
  title: "Locale Demand/WorldDemandMap",
  component: WorldDemandMap,
  decorators: [
    (Story) => (
      <div className="max-w-4xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WorldDemandMap>;

export const Default: Story = {
  args: { countries: snapshot.countries },
};

/** Brazil pre-hovered (world-atlas numeric id "076") so the tooltip shows. */
export const HoverTooltip: Story = {
  args: {
    countries: snapshot.countries,
    initialHoveredCountry: "076",
    selectedCountry: "BR",
  },
};
