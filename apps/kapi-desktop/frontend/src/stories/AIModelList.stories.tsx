import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AIModelList } from "../components/AIModelList";
import type { AIModelOption } from "../types/api";

const models: AIModelOption[] = [
  {
    model: "llama3.2:3b",
    provider: "ollama",
    label: "Ollama",
    local: true,
    installed: true,
    needs_key: false,
    note: "default · smallest, exact inline-tag fidelity",
    is_default: true,
  },
  {
    model: "gemma3:4b",
    provider: "ollama",
    label: "Ollama",
    local: true,
    installed: false,
    needs_key: false,
    note: "best multilingual quality · ~7 GB",
    is_default: false,
  },
  {
    model: "claude-sonnet-4-20250514",
    provider: "anthropic",
    label: "Anthropic",
    local: false,
    installed: false,
    needs_key: false,
    is_default: false,
  },
  {
    model: "gpt-4o",
    provider: "openai",
    label: "OpenAI",
    local: false,
    installed: false,
    needs_key: true,
    is_default: false,
  },
];

const meta: Meta<typeof AIModelList> = {
  title: "Components/AIModelList",
  component: AIModelList,
  tags: ["autodocs"],
  args: { models, onSelect: fn() },
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof AIModelList>;

/** Flat, model-first list with the provider label per row (the run-time prompt). */
export const Default: Story = {};

/** Grouped variant: the provider badge is hidden (a group header names it). */
export const Grouped: Story = { args: { showProvider: false } };

/** An explicit selection overrides the default-flagged row. */
export const Selected: Story = {
  args: { selected: { model: "gpt-4o", provider: "openai" } },
};

export const Empty: Story = { args: { models: [] } };
