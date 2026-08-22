import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ContextScanInput } from "../../context-scan/ContextScanInput";
import { withProviders } from "../decorators";

const meta: Meta<typeof ContextScanInput> = {
  title: "Context Scan/ContextScanInput",
  component: ContextScanInput,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 720, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ContextScanInput>;

/** Empty intake — the start button stays disabled until a source is added. */
export const Empty: Story = {
  args: {
    onStarted: fn(),
  },
};

/** Prefilled from a previous request (the regenerate path). */
export const Prefilled: Story = {
  args: {
    onStarted: fn(),
    initialRequest: {
      paste_text: "We build precise, dependable tools for translators.",
      urls: ["https://acme.example/about", "https://acme.example/blog/voice"],
      repo_url: "https://github.com/acme/website",
      profile_name: "Acme Voice",
      domain: "developer tools",
    },
  },
};
