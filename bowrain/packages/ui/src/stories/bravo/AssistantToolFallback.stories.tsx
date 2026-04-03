import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  ToolFallbackRoot,
  ToolFallbackTrigger,
  ToolFallbackContent,
  ToolFallbackArgs,
  ToolFallbackResult,
  ToolFallbackError,
} from "../../components/assistant-ui/tool-fallback";

/**
 * ToolFallback uses compound components (Root, Trigger, Content, Args, Result, Error)
 * that can be rendered standalone without the assistant-ui runtime.
 */

const meta: Meta = {
  title: "Bravo/Assistant UI/Tool Fallback",
  tags: ["autodocs"],
  parameters: {
    layout: "centered",
  },
  decorators: [
    (Story) => (
      <div className="w-[500px] bg-background text-foreground p-4">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj;

export const Complete: Story = {
  render: () => (
    <ToolFallbackRoot defaultOpen>
      <ToolFallbackTrigger toolName="run_flow" status={{ type: "complete" }} />
      <ToolFallbackContent>
        <ToolFallbackArgs
          argsText={JSON.stringify({ flow: "pseudo-translate", target_lang: "qps" }, null, 2)}
        />
        <ToolFallbackResult result={{ blocks_processed: 42, blocks_skipped: 3 }} />
      </ToolFallbackContent>
    </ToolFallbackRoot>
  ),
};

export const Running: Story = {
  render: () => (
    <ToolFallbackRoot>
      <ToolFallbackTrigger toolName="connector_push" status={{ type: "running" }} />
      <ToolFallbackContent>
        <ToolFallbackArgs
          argsText={JSON.stringify({ connector_id: "git-main", project_id: "proj-1" }, null, 2)}
        />
      </ToolFallbackContent>
    </ToolFallbackRoot>
  ),
};

export const RequiresAction: Story = {
  render: () => (
    <ToolFallbackRoot defaultOpen>
      <ToolFallbackTrigger
        toolName="connector_push"
        status={{ type: "requires-action", reason: "interrupt" }}
      />
      <ToolFallbackContent>
        <ToolFallbackArgs
          argsText={JSON.stringify({ connector_id: "git-main", project_id: "proj-1" }, null, 2)}
        />
      </ToolFallbackContent>
    </ToolFallbackRoot>
  ),
};

export const Cancelled: Story = {
  render: () => (
    <ToolFallbackRoot defaultOpen className="border-muted-foreground/30 bg-muted/30">
      <ToolFallbackTrigger
        toolName="run_flow"
        status={{ type: "incomplete", reason: "cancelled" }}
      />
      <ToolFallbackContent>
        <ToolFallbackError
          status={{
            type: "incomplete",
            reason: "cancelled",
            error: "User cancelled the operation",
          }}
        />
        <ToolFallbackArgs
          argsText={JSON.stringify({ flow: "pseudo-translate" }, null, 2)}
          className="opacity-60"
        />
      </ToolFallbackContent>
    </ToolFallbackRoot>
  ),
};

export const WithError: Story = {
  render: () => (
    <ToolFallbackRoot defaultOpen>
      <ToolFallbackTrigger toolName="run_flow" status={{ type: "incomplete", reason: "error" }} />
      <ToolFallbackContent>
        <ToolFallbackError
          status={{
            type: "incomplete",
            reason: "error",
            error: "Flow execution timed out after 30s",
          }}
        />
        <ToolFallbackArgs
          argsText={JSON.stringify({ flow: "translate", provider: "openai" }, null, 2)}
        />
      </ToolFallbackContent>
    </ToolFallbackRoot>
  ),
};

export const Collapsed: Story = {
  render: () => (
    <ToolFallbackRoot>
      <ToolFallbackTrigger toolName="run_flow" status={{ type: "complete" }} />
      <ToolFallbackContent>
        <ToolFallbackArgs argsText={JSON.stringify({ flow: "pseudo-translate" }, null, 2)} />
        <ToolFallbackResult result={{ blocks_processed: 10 }} />
      </ToolFallbackContent>
    </ToolFallbackRoot>
  ),
};
