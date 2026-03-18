import type { Meta, StoryObj } from "@storybook/react-vite";
import { CollectionHeatmap } from "../../components/CollectionHeatmap";
import { sampleCollectionStats } from "./fixtures";

const meta: Meta<typeof CollectionHeatmap> = {
  title: "Components/CollectionHeatmap",
  component: CollectionHeatmap,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 800, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof CollectionHeatmap>;

export const Default: Story = {
  args: { collectionStats: sampleCollectionStats, locales: ["fr-FR", "de-DE"] },
};
