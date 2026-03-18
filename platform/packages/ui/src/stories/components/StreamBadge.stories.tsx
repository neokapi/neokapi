import type { Meta, StoryObj } from "@storybook/react-vite";
import { StreamBadge } from "../../components/StreamBadge";
import { mainStream, featureStream, sharedStream } from "./fixtures";

const meta: Meta<typeof StreamBadge> = {
  title: "Components/StreamBadge",
  component: StreamBadge,
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
type Story = StoryObj<typeof StreamBadge>;

export const Public: Story = { args: { stream: mainStream } };
export const Private: Story = { args: { stream: featureStream } };
export const Shared: Story = { args: { stream: sharedStream } };
export const Compact: Story = { args: { stream: featureStream, compact: true } };
