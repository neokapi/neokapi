import type { Meta, StoryObj } from "@storybook/react-vite";
import { ModelUsageTable } from "../../components/billing/ModelUsageTable";
import type { ModelUsage, RunnerUsage } from "../../types/api";

const meta: Meta<typeof ModelUsageTable> = {
  title: "Billing/ModelUsageTable",
  component: ModelUsageTable,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof ModelUsageTable>;

const sampleEntries: ModelUsage[] = [
  {
    model: "claude-sonnet-4-20250514",
    operation: "translate",
    prompt_tokens: 245_000,
    output_tokens: 82_000,
    total_tokens: 327_000,
    call_count: 540,
  },
  {
    model: "gpt-4o",
    operation: "translate",
    prompt_tokens: 48_000,
    output_tokens: 15_000,
    total_tokens: 63_000,
    call_count: 95,
  },
  {
    model: "claude-sonnet-4-20250514",
    operation: "qa_check",
    prompt_tokens: 38_000,
    output_tokens: 9_500,
    total_tokens: 47_500,
    call_count: 72,
  },
  {
    model: "claude-sonnet-4-20250514",
    operation: "review",
    prompt_tokens: 12_000,
    output_tokens: 4_200,
    total_tokens: 16_200,
    call_count: 18,
  },
  {
    model: "gpt-4o",
    operation: "entity_extract",
    prompt_tokens: 8_400,
    output_tokens: 3_100,
    total_tokens: 11_500,
    call_count: 12,
  },
];

const sampleRunnerEntries: RunnerUsage[] = [
  { operation: "bravo_container", total_seconds: 2_340, count: 58 },
  { operation: "auto_translate", total_seconds: 845, count: 15 },
  { operation: "auto_extract", total_seconds: 120, count: 4 },
];

export const Default: Story = {
  args: {
    entries: sampleEntries,
    runnerEntries: sampleRunnerEntries,
  },
};

export const SingleModel: Story = {
  args: {
    entries: [
      {
        model: "claude-sonnet-4-20250514",
        operation: "translate",
        prompt_tokens: 1_200_000,
        output_tokens: 380_000,
        total_tokens: 1_580_000,
        call_count: 2_400,
      },
    ],
  },
};

export const Empty: Story = {
  args: {
    entries: [],
  },
};

export const RunnerOnly: Story = {
  args: {
    entries: [],
    runnerEntries: [
      { operation: "bravo_container", total_seconds: 7_200, count: 120 },
      { operation: "auto_translate", total_seconds: 3_600, count: 45 },
      { operation: "auto_extract", total_seconds: 180, count: 6 },
    ],
  },
};

export const TokensAndRunner: Story = {
  args: {
    entries: sampleEntries,
    runnerEntries: sampleRunnerEntries,
  },
};

export const HighVolume: Story = {
  args: {
    entries: [
      {
        model: "claude-sonnet-4-20250514",
        operation: "translate",
        prompt_tokens: 4_500_000,
        output_tokens: 1_200_000,
        total_tokens: 5_700_000,
        call_count: 8_500,
      },
      {
        model: "gpt-4o",
        operation: "translate",
        prompt_tokens: 2_100_000,
        output_tokens: 650_000,
        total_tokens: 2_750_000,
        call_count: 4_200,
      },
      {
        model: "claude-sonnet-4-20250514",
        operation: "qa_check",
        prompt_tokens: 890_000,
        output_tokens: 210_000,
        total_tokens: 1_100_000,
        call_count: 1_600,
      },
      {
        model: "claude-sonnet-4-20250514",
        operation: "brand_voice",
        prompt_tokens: 340_000,
        output_tokens: 95_000,
        total_tokens: 435_000,
        call_count: 520,
      },
    ],
  },
};
