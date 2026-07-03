import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConnectAICard } from "../components/ConnectAICard";
import type { AIDetectionResult } from "../types/api";

const meta: Meta<typeof ConnectAICard> = {
  title: "Home/ConnectAICard",
  component: ConnectAICard,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ConnectAICard>;

const DETECTED_BOTH: AIDetectionResult = {
  detected: [
    {
      provider: "claude-code",
      label: "Claude Code",
      model: "sonnet",
      detail: "signed in on this Mac · uses your Claude subscription",
      subscription: true,
    },
    {
      provider: "ollama",
      label: "Ollama",
      model: "llama3.2:3b",
      detail: "running locally · content stays on this machine",
      subscription: false,
    },
  ],
  configured: false,
};

const NOTHING_DETECTED: AIDetectionResult = {
  detected: [],
  configured: false,
};

/** Claude Code and Ollama both found: two one-click primary options, the
 *  API-key path and the demo escape hatch below. */
export const Detected: Story = {
  args: { detection: DETECTED_BOTH },
};

/** Nothing detected on the machine: the key-entry path leads. */
export const NothingDetected: Story = {
  args: { detection: NOTHING_DETECTED },
};

/** The expanded "I have an API key" form (click the toggle to reproduce
 *  interactively; this story starts from the nothing-detected state). */
export const KeyEntry: Story = {
  args: { detection: NOTHING_DETECTED },
  play: async ({ canvasElement }) => {
    const toggle = canvasElement.querySelector<HTMLButtonElement>(
      '[data-testid="connect-ai-key-toggle"]',
    );
    toggle?.click();
  },
};

/** Already configured: the card renders nothing (empty canvas by design). */
export const AlreadyConfigured: Story = {
  args: {
    detection: { detected: [], configured: true, default_provider: "claude-code" },
  },
};
