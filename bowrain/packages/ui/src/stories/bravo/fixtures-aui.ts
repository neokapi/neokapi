/**
 * Fixtures for assistant-ui powered Bravo Storybook stories.
 *
 * Provides helper functions to create mock ExternalStoreRuntime instances
 * that can be passed to <AssistantRuntimeProvider> in stories.
 */

import type { ThreadMessageLike } from "@assistant-ui/react";

// ---------------------------------------------------------------------------
// Sample assistant-ui messages
// ---------------------------------------------------------------------------

export const sampleAuiUserMessage: ThreadMessageLike = {
  role: "user",
  id: "msg-1",
  content: "Can you pseudo-translate the French files?",
  createdAt: new Date(Date.now() - 300000),
};

export const sampleAuiAssistantMessage: ThreadMessageLike = {
  role: "assistant",
  id: "msg-2",
  content: [
    {
      type: "text",
      text: 'Sure! I\'ll run the pseudo-translate flow on the French target files.\n\nHere\'s a quick script to verify the output:\n```python\nimport json\nwith open("fr-FR.json") as f:\n    data = json.load(f)\nprint(f"Keys: {len(data)}")\n```',
    },
    {
      type: "tool-call",
      toolCallId: "tc-1",
      toolName: "run_flow",
      args: { flow: "pseudo-translate", target_lang: "qps" },
      result: { blocks_processed: 42, blocks_skipped: 3 },
    },
  ],
  createdAt: new Date(Date.now() - 240000),
  status: { type: "complete", reason: "stop" },
  metadata: {
    custom: { input_tokens: 1500, output_tokens: 480 },
  },
};

export const sampleAuiApprovalMessage: ThreadMessageLike = {
  role: "assistant",
  id: "msg-3",
  content: [
    {
      type: "text",
      text: "Done! 42 blocks were pseudo-translated. I'd like to push the results to git \u2014 shall I?",
    },
    {
      type: "tool-call",
      toolCallId: "tc-2",
      toolName: "connector_push",
      args: { connector_id: "git-main", project_id: "proj-1" },
    },
  ],
  createdAt: new Date(Date.now() - 180000),
  status: { type: "requires-action", reason: "tool-calls" as const },
};

export const sampleAuiStreamingMessage: ThreadMessageLike = {
  role: "assistant",
  id: "__streaming__",
  content: [
    {
      type: "text",
      text: "I'm looking through your project files to find all translatable content...",
    },
  ],
  status: { type: "running" },
};

export const sampleAuiMessages: ThreadMessageLike[] = [
  sampleAuiUserMessage,
  sampleAuiAssistantMessage,
  sampleAuiApprovalMessage,
];

export const sampleAuiStreamingMessages: ThreadMessageLike[] = [
  sampleAuiUserMessage,
  sampleAuiStreamingMessage,
];

export const sampleAuiMarkdownMessage: ThreadMessageLike = {
  role: "assistant",
  id: "msg-md",
  content: [
    {
      type: "text",
      text: `## Translation Summary

Here's what I found in your project:

| File | Keys | Translated |
|------|------|-----------|
| en-US.json | 142 | \u2014 |
| fr-FR.json | 142 | 98 (69%) |
| de-DE.json | 142 | 45 (32%) |

### Key observations

1. **French** is mostly complete but has some missing keys
2. **German** needs significant work
3. All files use valid JSON with \`"key": "value"\` format

\`\`\`json
{
  "greeting": "Bonjour, {name}!",
  "farewell": "Au revoir!"
}
\`\`\`

> Note: The greeting key uses ICU message format for interpolation.`,
    },
  ],
  createdAt: new Date(Date.now() - 120000),
  status: { type: "complete", reason: "stop" },
};
