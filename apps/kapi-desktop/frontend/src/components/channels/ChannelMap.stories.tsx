import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ChannelMapRow } from "../../types/api";
import { ChannelMap } from "./ChannelMap";

const meta: Meta<typeof ChannelMap> = {
  title: "Channels/ChannelMap",
  component: ChannelMap,
  tags: ["autodocs"],
  args: { tabID: "t1" },
  render: (args) => (
    <div className="max-w-2xl">
      <ChannelMap {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof ChannelMap>;

const rows: ChannelMapRow[] = [
  {
    ref: "campaign/promo",
    profile: "campaign",
    channel: "promo",
    declared: true,
    voice: "Northsea",
    collections: ["Promo", "Spring"],
    item_count: 142,
  },
  {
    ref: "support/docs",
    profile: "support",
    channel: "docs",
    declared: true,
    voice: undefined,
    collections: ["Docs"],
    item_count: 12,
  },
  {
    ref: "blog/news",
    profile: "blog",
    channel: "news",
    declared: false,
    collections: ["News"],
    item_count: 3,
  },
];

export const Populated: Story = {
  name: "Declared, derived, and no voice",
  args: { channels: rows },
};

export const Empty: Story = {
  name: "No channels yet",
  args: { channels: [] },
};
