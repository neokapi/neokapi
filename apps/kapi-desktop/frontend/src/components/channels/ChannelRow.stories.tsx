import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { ChannelMapRow } from "../../types/api";
import { ChannelRow } from "./ChannelMap";

const meta: Meta<typeof ChannelRow> = {
  title: "Channels/ChannelRow",
  component: ChannelRow,
  tags: ["autodocs"],
  render: (args) => (
    <ul className="max-w-2xl divide-y">
      <ChannelRow {...args} />
    </ul>
  ),
};

export default meta;
type Story = StoryObj<typeof ChannelRow>;

const declared: ChannelMapRow = {
  ref: "campaign/promo",
  profile: "campaign",
  channel: "promo",
  declared: true,
  voice: "Northsea",
  collections: ["Promo"],
  item_count: 142,
};

export const Declared: Story = {
  name: "Declared (renameable)",
  args: { channel: declared, onRename: fn() },
};

export const NoVoiceProfile: Story = {
  name: "No voice profile",
  args: { channel: { ...declared, voice: undefined, item_count: 0 }, onRename: fn() },
};

export const Derived: Story = {
  name: "Derived (read-only)",
  args: {
    channel: {
      ref: "blog/news",
      profile: "blog",
      channel: "news",
      declared: false,
      collections: ["News"],
      item_count: 3,
    },
  },
};
