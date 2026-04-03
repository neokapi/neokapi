import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  useExternalStoreRuntime,
  AssistantRuntimeProvider,
  type ThreadMessageLike,
} from "@assistant-ui/react";
import { Thread } from "../../components/assistant-ui/thread";

// ---------------------------------------------------------------------------
// Wrapper that provides a runtime with attachment messages
// ---------------------------------------------------------------------------

function ThreadWithAttachments({
  messages,
  isRunning = false,
}: {
  messages: ThreadMessageLike[];
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
        <Thread />
      </div>
    </AssistantRuntimeProvider>
  );
}

// ---------------------------------------------------------------------------
// Sample messages with attachments
// ---------------------------------------------------------------------------

const userMessageWithImageAttachment: ThreadMessageLike = {
  role: "user",
  id: "msg-attach-1",
  content: "Can you translate the text in this screenshot?",
  attachments: [
    {
      id: "att-1",
      type: "image",
      name: "screenshot.png",
      status: { type: "complete" },
      content: [
        {
          type: "image",
          image:
            "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='200' height='100'%3E%3Crect width='200' height='100' fill='%23e2e8f0'/%3E%3Ctext x='50%25' y='50%25' dominant-baseline='middle' text-anchor='middle' font-family='sans-serif' fill='%2364748b'%3EScreenshot%3C/text%3E%3C/svg%3E",
        },
      ],
    },
  ],
  createdAt: new Date(Date.now() - 120000),
};

const userMessageWithFileAttachment: ThreadMessageLike = {
  role: "user",
  id: "msg-attach-2",
  content: "Please translate this JSON file.",
  attachments: [
    {
      id: "att-2",
      type: "document",
      name: "en-US.json",
      status: { type: "complete" },
      content: [
        {
          type: "text",
          text: '{"greeting": "Hello", "farewell": "Goodbye"}',
        },
      ],
    },
  ],
  createdAt: new Date(Date.now() - 60000),
};

const assistantReply: ThreadMessageLike = {
  role: "assistant",
  id: "msg-attach-reply",
  content: [{ type: "text", text: "I can see the file. Let me translate it for you." }],
  createdAt: new Date(Date.now() - 30000),
  status: { type: "complete", reason: "stop" },
};

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta = {
  title: "Bravo/Assistant UI/Attachment",
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

export const ImageAttachment: Story = {
  render: () => (
    <ThreadWithAttachments messages={[userMessageWithImageAttachment, assistantReply]} />
  ),
};

export const FileAttachment: Story = {
  render: () => (
    <ThreadWithAttachments messages={[userMessageWithFileAttachment, assistantReply]} />
  ),
};
