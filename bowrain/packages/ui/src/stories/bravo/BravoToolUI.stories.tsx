import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoToolCallRenderer } from "../../components/bravo/bravo-tool-ui";

const meta: Meta<typeof BravoToolCallRenderer> = {
  title: "Bravo/BravoToolUI",
  component: BravoToolCallRenderer,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 480, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BravoToolCallRenderer>;

export const Completed: Story = {
  args: {
    toolName: "translate_block",
    toolCallId: "call_001",
    args: {
      block_id: "seg-42",
      source: "Hello, world!",
      target_locale: "fr-FR",
    },
    result: { translation: "Bonjour le monde !" },
    status: { type: "complete" },
  },
};

export const Running: Story = {
  args: {
    toolName: "quality_check",
    toolCallId: "call_002",
    args: {
      file: "messages.json",
      locale: "de-DE",
    },
    status: { type: "running" },
  },
};

export const Error: Story = {
  args: {
    toolName: "push_connector",
    toolCallId: "call_003",
    args: {
      connector: "github",
      branch: "l10n/main",
    },
    result: { error: "Authentication failed: token expired" },
    isError: true,
    status: { type: "error" },
  },
};

export const RequiresApproval: Story = {
  args: {
    toolName: "delete_translations",
    toolCallId: "call_004",
    args: {
      locale: "ja-JP",
      count: 47,
    },
    status: { type: "requires-action" },
    addResult: fn(),
  },
};

export const Pending: Story = {
  args: {
    toolName: "batch_translate",
    toolCallId: "call_005",
    args: {
      source_locale: "en-US",
      target_locales: ["fr-FR", "de-DE", "es-ES"],
    },
    status: { type: "pending" },
  },
};
