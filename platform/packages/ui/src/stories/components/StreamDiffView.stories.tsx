import type { Meta, StoryObj } from "@storybook/react-vite";
import { StreamDiffView } from "../../components/StreamDiffView";
import { sampleDiff, emptyDiff } from "./fixtures";

const meta: Meta<typeof StreamDiffView> = {
  title: "Components/StreamDiffView",
  component: StreamDiffView,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 640, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof StreamDiffView>;

export const WithChanges: Story = { args: { diff: sampleDiff } };
export const NoDifferences: Story = { args: { diff: emptyDiff } };
