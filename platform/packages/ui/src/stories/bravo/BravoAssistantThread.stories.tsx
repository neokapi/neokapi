import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  useExternalStoreRuntime,
  AssistantRuntimeProvider,
  type ThreadMessageLike,
} from "@assistant-ui/react";
import { BravoAssistantThread } from "../../components/bravo/bravo-thread";
import {
  sampleAuiMessages,
  sampleAuiStreamingMessages,
  sampleAuiMarkdownMessage,
  sampleAuiUserMessage,
} from "./fixtures-aui";

// ---------------------------------------------------------------------------
// Wrapper that provides a mock runtime
// ---------------------------------------------------------------------------

function ThreadWithRuntime({
  messages = [],
  isRunning = false,
}: {
  messages?: ThreadMessageLike[];
  isRunning?: boolean;
}) {
  const runtime = useExternalStoreRuntime({
    messages,
    isRunning,
    convertMessage: (msg: ThreadMessageLike) => msg,
    onNew: async () => {},
    onCancel: async () => {},
  });

  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <div className="h-[500px] w-[400px] border rounded-lg overflow-hidden flex flex-col bg-background text-foreground">
        <BravoAssistantThread />
      </div>
    </AssistantRuntimeProvider>
  );
}

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta = {
  title: "Bravo/BravoAssistantThread",
  tags: ["autodocs"],
  parameters: {
    layout: "centered",
  },
};

export default meta;
type Story = StoryObj;

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

export const WithMessages: Story = {
  render: () => <ThreadWithRuntime messages={sampleAuiMessages} />,
};

export const Streaming: Story = {
  render: () => <ThreadWithRuntime messages={sampleAuiStreamingMessages} isRunning />,
};

export const RichMarkdown: Story = {
  render: () => (
    <ThreadWithRuntime messages={[sampleAuiUserMessage, sampleAuiMarkdownMessage]} />
  ),
};

export const Empty: Story = {
  render: () => <ThreadWithRuntime />,
};
