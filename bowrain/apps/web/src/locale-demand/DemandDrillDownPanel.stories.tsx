import type { Meta, StoryObj } from "@storybook/react-vite";
import { DemandDrillDownPanel } from "./DemandDrillDownPanel";
import { sampleDemandDataSource } from "./locale-demand-fixtures";

// Locale demand — DESIGN PROTOTYPE (mock data only).

const snapshot = sampleDemandDataSource.getSnapshot("30d");

const meta: Meta<typeof DemandDrillDownPanel> = {
  title: "Locale Demand/DemandDrillDownPanel",
  component: DemandDrillDownPanel,
  decorators: [
    (Story) => (
      <div className="p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof DemandDrillDownPanel>;

/** Drill-down after clicking a country on the map. */
export const Country: Story = {
  args: {
    snapshot,
    selection: { kind: "country", code: "BR" },
    onClose: () => {},
  },
};

/** Drill-down after clicking a language row in the table. */
export const Language: Story = {
  args: {
    snapshot,
    selection: { kind: "language", code: "ko" },
    onClose: () => {},
  },
};
