import type { Meta, StoryObj } from "@storybook/react-vite";
import { StreamTagBadge } from "../../components/StreamTagBadge";
import { mergeTag, releaseTag, milestoneTag, customTag } from "./fixtures";

const meta: Meta<typeof StreamTagBadge> = {
  title: "Streams/Tags/StreamTagBadge",
  component: StreamTagBadge,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 24, display: "flex", gap: 16, alignItems: "center" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof StreamTagBadge>;

export const Merge: Story = { args: { tag: mergeTag } };
export const Release: Story = { args: { tag: releaseTag } };
export const Milestone: Story = { args: { tag: milestoneTag } };
export const Custom: Story = { args: { tag: customTag } };
export const CompactMerge: Story = { args: { tag: mergeTag, compact: true } };
export const CompactRelease: Story = { args: { tag: releaseTag, compact: true } };
