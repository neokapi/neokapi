import type { Meta, StoryObj } from "@storybook/react-vite";
import { StreamTagList } from "../../components/StreamTagList";
import { sampleTags, mergeTag } from "./fixtures";

const meta: Meta<typeof StreamTagList> = {
  title: "Streams/Tags/StreamTagList",
  component: StreamTagList,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 24, maxWidth: 600 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof StreamTagList>;

export const Default: Story = { args: { tags: sampleTags } };

export const WithDelete: Story = {
  args: { tags: sampleTags, onDelete: (name: string) => alert(`Delete: ${name}`) },
};

export const Empty: Story = { args: { tags: [] } };

export const SingleMerge: Story = { args: { tags: [mergeTag] } };
