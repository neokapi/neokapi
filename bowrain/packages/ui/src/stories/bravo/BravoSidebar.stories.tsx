import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { useExternalStoreRuntime, type ThreadMessageLike } from "@assistant-ui/react";
import { BravoSidebar } from "../../components/bravo/bravo-sidebar";
import { sampleConversations } from "./fixtures";
import {
  sampleAuiMessages,
  sampleAuiStreamingMessages,
  sampleAuiMarkdownMessage,
  sampleAuiUserMessage,
} from "./fixtures-aui";

// ---------------------------------------------------------------------------
// Helper: creates a mock assistant-ui runtime for stories
// ---------------------------------------------------------------------------

function MockRuntimeWrapper({
  messages = [],
  isRunning = false,
  children,
}: {
  messages?: ThreadMessageLike[];
  isRunning?: boolean;
  children: (runtime: ReturnType<typeof useExternalStoreRuntime>) => React.ReactNode;
}) {
  const runtime = useExternalStoreRuntime({
    messages,
    isRunning,
    convertMessage: (msg: ThreadMessageLike) => msg,
    onNew: async () => {},
    onCancel: async () => {},
  });

  return <>{children(runtime)}</>;
}

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta<typeof BravoSidebar> = {
  title: "Bravo/BravoSidebar",
  component: BravoSidebar,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof BravoSidebar>;

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

export const ConversationList: Story = {
  render: () => (
    <MockRuntimeWrapper>
      {(runtime) => (
        <BravoSidebar
          open
          onOpenChange={fn()}
          runtime={runtime}
          view="list"
          onBack={fn()}
          conversationListProps={{
            conversations: sampleConversations,
            onSelect: fn(),
            onDelete: fn(),
            onNew: fn(),
          }}
        />
      )}
    </MockRuntimeWrapper>
  ),
};

export const ActiveChat: Story = {
  render: () => (
    <MockRuntimeWrapper messages={sampleAuiMessages}>
      {(runtime) => (
        <BravoSidebar
          open
          onOpenChange={fn()}
          runtime={runtime}
          view="chat"
          onBack={fn()}
          activeTitle="Translate French files"
          mode="ask"
          onModeChange={fn()}
          onShowConfig={fn()}
        />
      )}
    </MockRuntimeWrapper>
  ),
};

export const Streaming: Story = {
  render: () => (
    <MockRuntimeWrapper messages={sampleAuiStreamingMessages} isRunning>
      {(runtime) => (
        <BravoSidebar
          open
          onOpenChange={fn()}
          runtime={runtime}
          view="chat"
          onBack={fn()}
          activeTitle="Translate French files"
          mode="coworker"
          onModeChange={fn()}
        />
      )}
    </MockRuntimeWrapper>
  ),
};

export const RichMarkdown: Story = {
  render: () => (
    <MockRuntimeWrapper messages={[sampleAuiUserMessage, sampleAuiMarkdownMessage]}>
      {(runtime) => (
        <BravoSidebar
          open
          onOpenChange={fn()}
          runtime={runtime}
          view="chat"
          onBack={fn()}
          activeTitle="Project analysis"
          mode="ask"
          onModeChange={fn()}
        />
      )}
    </MockRuntimeWrapper>
  ),
};

export const ColdStart: Story = {
  render: () => (
    <MockRuntimeWrapper messages={[sampleAuiUserMessage]}>
      {(runtime) => (
        <BravoSidebar
          open
          onOpenChange={fn()}
          runtime={runtime}
          view="chat"
          onBack={fn()}
          activeTitle="New conversation"
          coldStarting
        />
      )}
    </MockRuntimeWrapper>
  ),
};

export const Empty: Story = {
  render: () => (
    <MockRuntimeWrapper>
      {(runtime) => (
        <BravoSidebar
          open
          onOpenChange={fn()}
          runtime={runtime}
          view="chat"
          onBack={fn()}
          mode="ask"
          onModeChange={fn()}
        />
      )}
    </MockRuntimeWrapper>
  ),
};
