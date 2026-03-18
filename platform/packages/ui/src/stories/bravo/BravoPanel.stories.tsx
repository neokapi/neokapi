import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoPanel } from "../../components/bravo/BravoPanel";
import { sampleConversations, sampleMessages } from "./fixtures";

const meta: Meta<typeof BravoPanel> = {
  title: "Bravo/BravoPanel",
  component: BravoPanel,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof BravoPanel>;

export const ConversationList: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    conversations: sampleConversations,
    messages: [],
    onNewConversation: fn(),
    onSelectConversation: fn(),
    onDeleteConversation: fn(),
    onSendMessage: fn(),
    onApproveToolCall: fn(),
    onDenyToolCall: fn(),
    onShowConfig: fn(),
  },
};

export const ActiveChat: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    conversations: sampleConversations,
    activeConversation: sampleConversations[0],
    messages: sampleMessages,
    onNewConversation: fn(),
    onSelectConversation: fn(),
    onDeleteConversation: fn(),
    onSendMessage: fn(),
    onApproveToolCall: fn(),
    onDenyToolCall: fn(),
    onShowConfig: fn(),
  },
};

export const Streaming: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    conversations: sampleConversations,
    activeConversation: sampleConversations[0],
    messages: sampleMessages.slice(0, 1),
    streaming: true,
    streamingContent: "I'm looking through your project files to find all translatable content...",
    onNewConversation: fn(),
    onSelectConversation: fn(),
    onDeleteConversation: fn(),
    onSendMessage: fn(),
    onApproveToolCall: fn(),
    onDenyToolCall: fn(),
  },
};

export const Empty: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    conversations: [],
    messages: [],
    onNewConversation: fn(),
    onSelectConversation: fn(),
    onDeleteConversation: fn(),
    onSendMessage: fn(),
    onApproveToolCall: fn(),
    onDenyToolCall: fn(),
  },
};
