import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { CollectionTabs } from "../../components/CollectionTabs";
import { sampleCollections, defaultCollection } from "./fixtures";

const meta: Meta<typeof CollectionTabs> = {
  title: "Workspace/Collections/CollectionTabs",
  component: CollectionTabs,
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
type Story = StoryObj<typeof CollectionTabs>;

export const MultipleCollections: Story = {
  args: {
    collections: sampleCollections,
    activeCollectionId: defaultCollection.id,
    onSelectCollection: fn(),
    onCreateCollection: fn(),
    onEditCollection: fn(),
    onDeleteCollection: fn(),
  },
};

export const SingleCollection: Story = {
  args: {
    collections: [defaultCollection],
    activeCollectionId: defaultCollection.id,
    onSelectCollection: fn(),
    onCreateCollection: fn(),
  },
};
