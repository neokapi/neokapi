import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoConversationList } from "../../components/bravo/BravoConversationList";
import { sampleConversations } from "./fixtures";

const meta: Meta<typeof BravoConversationList> = {
  title: "Bravo/BravoConversationList",
  component: BravoConversationList,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 400, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BravoConversationList>;

export const Default: Story = {
  args: {
    conversations: sampleConversations,
    onSelect: fn(),
    onDelete: fn(),
    onNew: fn(),
  },
};

export const WithActive: Story = {
  args: {
    conversations: sampleConversations,
    activeId: "conv-1",
    onSelect: fn(),
    onDelete: fn(),
    onNew: fn(),
  },
};

export const Empty: Story = {
  args: {
    conversations: [],
    onSelect: fn(),
    onNew: fn(),
  },
};

export const Loading: Story = {
  args: {
    conversations: [],
    loading: true,
    onSelect: fn(),
    onNew: fn(),
  },
};
