import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ErrorProvider } from "../components/ErrorBanner";
import { ChecksPanel } from "../components/ChecksPanel";
import type { CheckRunResult, DesktopFinding } from "../types/api";

const PASSING: CheckRunResult = {
  pass: true,
  score: 100,
  files: [{ path: "src/locales/en.json", findings: [] }],
};

const FAILING: CheckRunResult = {
  pass: false,
  score: 58,
  files: [
    {
      path: "src/locales/en.json",
      findings: [
        {
          category: "do-not-translate",
          severity: "critical",
          message:
            'Do-not-translate term "Acme Cloud" is missing from the de target — it appears to have been translated or altered',
          suggestion: 'Keep "Acme Cloud" verbatim in the target',
          original_text: "Acme Cloud",
          block_id: "blk-1",
          field: "target",
          fixable: false,
        },
        {
          category: "vocabulary",
          severity: "major",
          message: 'Forbidden term "utilize" found',
          suggestion: 'Use "use" instead',
          original_text: "utilize",
          replacement: "use",
          block_id: "blk-2",
          field: "source",
          fixable: true,
        },
        {
          category: "vocabulary",
          severity: "minor",
          message: 'Forbidden term "leverage" found',
          suggestion: 'Use "use" instead',
          original_text: "leverage",
          replacement: "use",
          block_id: "blk-3",
          field: "source",
          fixable: true,
        },
      ],
    },
    {
      path: "src/locales/de.json",
      findings: [
        {
          category: "placeholder",
          severity: "critical",
          message: "Placeholder {count} is missing from the de target",
          original_text: "{count}",
          block_id: "blk-4",
          field: "target",
          fixable: false,
        },
        {
          category: "register",
          severity: "neutral",
          message: "Tone reads more formal than the brand's casual register",
          block_id: "blk-5",
          field: "source",
          fixable: false,
        },
      ],
    },
  ],
};

const meta: Meta<typeof ChecksPanel> = {
  title: "Pages/ChecksPanel",
  component: ChecksPanel,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <ErrorProvider>
        <div style={{ height: 760 }}>
          <Story />
        </div>
      </ErrorProvider>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "Runs content checks (do-not-translate, placeholder integrity, brand vocabulary) over a project's files like tests over code, grouped by file and severity, with a one-click fix for findings that carry a safe structured replacement.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ChecksPanel>;

/** A clean run — everything passes. */
export const Passing: Story = {
  args: { tabID: "story", result: PASSING, targetLanguages: ["de", "fr"] },
};

/** A failing run with mixed severities and a couple of fixable findings. */
export const Failing: Story = {
  args: { tabID: "story", result: FAILING, targetLanguages: ["de", "fr"] },
};

/** The loading/skeleton state while a run is in flight. */
export const Loading: Story = {
  args: { tabID: "story", forceLoading: true, targetLanguages: ["de"] },
};

/**
 * Interactive: applying a fix removes the finding and recomputes the score.
 * Wires onApplyFix to a local reducer so the story behaves like the real panel
 * without a Wails backend.
 */
export const InteractiveFix: StoryObj<typeof ChecksPanel> = {
  render: () => {
    function Wrapper() {
      const [result, setResult] = useState<CheckRunResult>(FAILING);
      const applyFix = async (filePath: string, finding: DesktopFinding) => {
        setResult((prev) => {
          const files = prev.files.map((f) =>
            f.path === filePath
              ? { ...f, findings: f.findings.filter((x) => x.block_id !== finding.block_id) }
              : f,
          );
          const remaining = files.flatMap((f) => f.findings);
          const critical = remaining.some((f) => f.severity === "critical");
          // Crude score: 100 − Σ MQM-ish penalties.
          const weight = (s: string) =>
            s === "critical" ? 25 : s === "major" ? 5 : s === "minor" ? 1 : 0;
          const score = Math.max(0, 100 - remaining.reduce((n, f) => n + weight(f.severity), 0));
          return { pass: !critical, score, files };
        });
      };
      return (
        <ChecksPanel tabID="story" result={result} targetLanguages={["de"]} onApplyFix={applyFix} />
      );
    }
    return <Wrapper />;
  },
};
