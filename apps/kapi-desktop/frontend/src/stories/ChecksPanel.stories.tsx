import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { ContentTree } from "@neokapi/ui-primitives/preview";
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
            'Do-not-translate term "Acme Cloud" is missing from the de target: it appears to have been translated or altered',
          suggestion: 'Keep "Acme Cloud" verbatim in the target',
          original_text: "Acme Cloud",
          block_id: "blk-1",
          field: "target",
          locale: "de",
          fixable: false,
          position: { kind: "range", start: { run: 0, offset: 11 }, end: { run: 0, offset: 21 } },
          source_runs: [{ text: "Welcome to Acme Cloud, where your team ships faster." }],
          target_runs: [{ text: "Willkommen bei Acme Wolke, wo Ihr Team schneller liefert." }],
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
          locale: "en",
          fixable: true,
          position: { kind: "range", start: { run: 0, offset: 7 }, end: { run: 0, offset: 14 } },
          source_runs: [{ text: "Please utilize the dashboard to review your credits." }],
        },
        {
          category: "vocabulary",
          severity: "minor",
          message: 'Prefer "overview" to "dashboard" in product copy',
          suggestion: 'Use "overview" instead',
          original_text: "dashboard",
          replacement: "overview",
          block_id: "blk-2",
          field: "source",
          locale: "en",
          fixable: true,
          position: { kind: "range", start: { run: 0, offset: 19 }, end: { run: 0, offset: 28 } },
          source_runs: [{ text: "Please utilize the dashboard to review your credits." }],
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
          locale: "en",
          fixable: true,
          position: { kind: "range", start: { run: 2, offset: 0 }, end: { run: 2, offset: 8 } },
          source_runs: [
            { text: "Your credits reset on " },
            { ph: { id: "date", type: "var", data: "{date}", equiv: "date" } },
            { text: "leverage them before then." },
          ],
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
          locale: "de",
          fixable: false,
          source_runs: [
            { text: "You have " },
            { ph: { id: "count", type: "var", data: "{count}", equiv: "count" } },
            { text: " new messages" },
          ],
          target_runs: [{ text: "Sie haben neue Nachrichten" }],
        },
        {
          category: "register",
          severity: "neutral",
          message: "Tone reads more formal than the brand's casual register",
          block_id: "blk-5",
          field: "source",
          locale: "en",
          fixable: false,
          source_runs: [
            { text: "Kindly proceed to the billing area at your earliest convenience." },
          ],
        },
      ],
    },
  ],
};

/**
 * The checked file as InspectFileAnnotated returns it, so a finding can be
 * opened in its document without a backend. The blocks are named by the
 * reader's key path, which is how the preview addresses a unit; the findings
 * name them by id.
 */
const CHECKED_FILE: ContentTree = {
  format: "json",
  root: [
    {
      kind: "block",
      id: "blk-1",
      name: "home.welcome",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Welcome to Acme Cloud, where your team ships faster." }],
      targets: { de: [{ text: "Willkommen bei Acme Wolke, wo Ihr Team schneller liefert." }] },
    },
    {
      kind: "block",
      id: "blk-2",
      name: "home.credits",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Please utilize the dashboard to review your credits." }],
      targets: { de: [{ text: "Bitte nutzen Sie das Dashboard, um Ihr Guthaben zu prüfen." }] },
    },
    {
      kind: "block",
      id: "blk-3",
      name: "home.reset",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [
        { text: "Your credits reset on " },
        { ph: { id: "date", type: "var", data: "{date}", equiv: "date" } },
        { text: "leverage them before then." },
      ],
      targets: {
        de: [
          { text: "Ihr Guthaben wird am " },
          { ph: { id: "date", type: "var", data: "{date}", equiv: "date" } },
          { text: " zurückgesetzt." },
        ],
      },
    },
  ],
  stats: { layers: 0, groups: 0, blocks: 3, data: 0, media: 0, runs: 8 },
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
  args: { tabID: "story", result: PASSING },
};

/**
 * A failing run with mixed severities and a couple of fixable findings. Each
 * card reads its finding in the text it was raised on with the span marked; a
 * finding about the translation reads the target first and the source beneath
 * it with the words underlined, and one block carries two findings.
 */
export const Failing: Story = {
  args: { tabID: "story", result: FAILING, previewTree: CHECKED_FILE },
};
export const FailingDark: Story = {
  args: Failing.args,
  globals: { theme: "dark" },
};

/** The loading/skeleton state while a run is in flight. */
export const Loading: Story = {
  args: { tabID: "story", forceLoading: true },
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
      return <ChecksPanel tabID="story" result={result} onApplyFix={applyFix} />;
    }
    return <Wrapper />;
  },
};
