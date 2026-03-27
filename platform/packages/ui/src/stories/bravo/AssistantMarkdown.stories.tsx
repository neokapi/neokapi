import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  useExternalStoreRuntime,
  AssistantRuntimeProvider,
  type ThreadMessageLike,
} from "@assistant-ui/react";
import { Thread } from "../../components/assistant-ui/thread";

// ---------------------------------------------------------------------------
// Wrapper
// ---------------------------------------------------------------------------

function ThreadWithMarkdown({ messages }: { messages: ThreadMessageLike[] }) {
  const runtime = useExternalStoreRuntime({
    messages,
    isRunning: false,
    convertMessage: (msg: ThreadMessageLike) => msg,
    onNew: async () => {},
    onCancel: async () => {},
  });

  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <div className="h-[600px] w-[500px] border rounded-lg overflow-hidden flex flex-col bg-background text-foreground">
        <Thread />
      </div>
    </AssistantRuntimeProvider>
  );
}

// ---------------------------------------------------------------------------
// Sample messages
// ---------------------------------------------------------------------------

const userMsg: ThreadMessageLike = {
  role: "user",
  id: "md-user",
  content: "Show me what the markdown rendering looks like.",
  createdAt: new Date(Date.now() - 60000),
};

const headingsMsg: ThreadMessageLike = {
  role: "assistant",
  id: "md-headings",
  content: [
    {
      type: "text",
      text: `# Heading 1
## Heading 2
### Heading 3
#### Heading 4

Regular paragraph text with **bold**, *italic*, and \`inline code\`.`,
    },
  ],
  status: { type: "complete", reason: "stop" },
};

const codeBlockMsg: ThreadMessageLike = {
  role: "assistant",
  id: "md-code",
  content: [
    {
      type: "text",
      text: `Here is a code example:

\`\`\`typescript
interface TranslationEntry {
  key: string;
  source: string;
  target?: string;
  status: "translated" | "untranslated" | "fuzzy";
}

function getUntranslated(entries: TranslationEntry[]): TranslationEntry[] {
  return entries.filter(e => e.status === "untranslated");
}
\`\`\`

And some inline code: \`getUntranslated(entries)\``,
    },
  ],
  status: { type: "complete", reason: "stop" },
};

const tableMsg: ThreadMessageLike = {
  role: "assistant",
  id: "md-table",
  content: [
    {
      type: "text",
      text: `## Translation Progress

| Language | Keys | Translated | Coverage |
|----------|------|-----------|----------|
| English  | 142  | 142       | 100%     |
| French   | 142  | 98        | 69%      |
| German   | 142  | 45        | 32%      |
| Japanese | 142  | 12        | 8%       |

> Note: Coverage below 50% is flagged for review.`,
    },
  ],
  status: { type: "complete", reason: "stop" },
};

const listsMsg: ThreadMessageLike = {
  role: "assistant",
  id: "md-lists",
  content: [
    {
      type: "text",
      text: `### Unordered list
- First item
- Second item with **bold**
- Third item with \`code\`

### Ordered list
1. Parse source files
2. Extract translatable strings
3. Send to translation provider
4. Merge results back

---

Here is a [link to docs](https://example.com) and a blockquote:

> Localization is not just translation. It requires cultural adaptation.`,
    },
  ],
  status: { type: "complete", reason: "stop" },
};

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta = {
  title: "Bravo/Assistant UI/Markdown",
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

export const Headings: Story = {
  render: () => <ThreadWithMarkdown messages={[userMsg, headingsMsg]} />,
};

export const CodeBlocks: Story = {
  render: () => <ThreadWithMarkdown messages={[userMsg, codeBlockMsg]} />,
};

export const Tables: Story = {
  render: () => <ThreadWithMarkdown messages={[userMsg, tableMsg]} />,
};

export const Lists: Story = {
  render: () => <ThreadWithMarkdown messages={[userMsg, listsMsg]} />,
};

export const AllElements: Story = {
  render: () => (
    <ThreadWithMarkdown messages={[userMsg, headingsMsg, codeBlockMsg, tableMsg, listsMsg]} />
  ),
};
