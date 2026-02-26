import type { Meta, StoryObj } from "@storybook/react";
import { fn } from "@storybook/test";
import { ProblemsPanel } from "../../components/editor/ProblemsPanel";
import { sampleFileQAResults } from "../fixtures";

const meta: Meta<typeof ProblemsPanel> = {
  title: "Editor/ProblemsPanel",
  component: ProblemsPanel,
  tags: ["autodocs"],
  args: {
    onNavigateToBlock: fn(),
    onClose: fn(),
  },
  parameters: {
    layout: "fullscreen",
  },
  decorators: [
    (Story) => (
      <div style={{ height: "100vh", position: "relative" }}>
        <div style={{ padding: 24, color: "var(--foreground)" }}>
          <p>Document content above the problems panel...</p>
        </div>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ProblemsPanel>;

/** Panel with mixed errors and warnings */
export const WithIssues: Story = {
  args: {
    issues: sampleFileQAResults,
  },
};

/** No issues found — shows success state */
export const NoIssues: Story = {
  args: {
    issues: [],
  },
};

/** Loading state while QA checks are running */
export const Loading: Story = {
  args: {
    issues: [],
    loading: true,
  },
};

/** Errors only — multiple error-severity issues */
export const ErrorsOnly: Story = {
  args: {
    issues: [
      {
        blockId: "blk-2",
        issues: [
          { type: "missing-tag", severity: "error", message: 'Missing closing <b> tag in target' },
        ],
      },
      {
        blockId: "blk-6",
        issues: [
          { type: "placeholder", severity: "error", message: "Missing placeholder {count} in target" },
          { type: "punctuation", severity: "error", message: 'Target ends with "." but source does not' },
        ],
      },
    ],
  },
};

/** Warnings only — no errors */
export const WarningsOnly: Story = {
  args: {
    issues: [
      {
        blockId: "blk-1",
        issues: [
          { type: "terminology", severity: "warning", message: '"localization" should be "localisation"' },
        ],
      },
      {
        blockId: "blk-3",
        issues: [
          { type: "whitespace", severity: "warning", message: "Trailing whitespace in target" },
          { type: "capitalization", severity: "warning", message: "Target starts with lowercase but source starts with uppercase" },
        ],
      },
    ],
  },
};

/** Many issues — tests scroll behavior */
export const ManyIssues: Story = {
  args: {
    issues: Array.from({ length: 10 }, (_, i) => ({
      blockId: `blk-${i + 1}`,
      issues: [
        { type: "tag-mismatch", severity: (i % 3 === 0 ? "error" : "warning") as "error" | "warning", message: `Issue in block ${i + 1}: tag mismatch detected` },
        { type: "length", severity: "warning" as const, message: `Block ${i + 1}: target is 40% longer than source` },
      ],
    })),
  },
};
