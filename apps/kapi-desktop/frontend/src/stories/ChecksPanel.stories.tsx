import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { ContentTree } from "@neokapi/ui-primitives/preview";
import { ErrorProvider } from "../components/ErrorBanner";
import { ChecksPanel } from "../components/ChecksPanel";
import type { CheckRunResult } from "../types/api";
import type { ChangeClient } from "../lib/changes";
import { MemoryChanges, type MemoryBlock } from "./memoryChanges";

/** The checked file's blocks as the change service reads them. */
const CHECKED_BLOCKS: MemoryBlock[] = [
  {
    doc: "src/locales/en.json",
    block: "blk-2",
    text: "Please utilize the dashboard to review your credits.",
  },
  {
    doc: "src/locales/en.json",
    block: "blk-3",
    text: 'Your credits reset on <x id="date/"/>leverage them before then.',
    codes: { "date/": { kind: "placeholder", type: "var", equiv: "date" } },
  },
];
const READER = new MemoryChanges(CHECKED_BLOCKS);

/** A finding's fix: the replace_text the check carries, under the revision it read. */
function fixFor(block: string, start: number, end: number, run: number, text: string): string {
  const b = CHECKED_BLOCKS.find((x) => x.block === block)!;
  return JSON.stringify({
    op: "replace_text",
    at: { doc: "src/locales/en.json", block },
    if_match: READER.rev(b),
    edits: [{ range: { start: { run, offset: start }, end: { run, offset: end } }, text }],
  });
}

const PASSING: CheckRunResult = {
  pass: true,
  verdict: "passed",
  score: 100,
  files: [{ path: "src/locales/en.json", findings: [] }],
};

const PASSING_WITH_WARNINGS: CheckRunResult = {
  ...PASSING,
  warnings: [
    {
      code: "voice.unknown_key",
      message:
        'unknown key "vocab" (line 9) is ignored when the profile loads; check its spelling and the section it sits under',
      source: ".kapi/voice.yaml",
      key: "channels.docs.vocabulary",
    },
  ],
};

const NOTHING_TO_CHECK: CheckRunResult = {
  pass: false,
  verdict: "did_not_run",
  did_not_run_cause: "nothing_to_check",
  did_not_run: ["no content blocks were checked"],
  score: 100,
  files: [{ path: "src/locales/en.json", findings: [] }],
};

const CHECKER_INVALID: CheckRunResult = {
  pass: false,
  verdict: "did_not_run",
  did_not_run_cause: "checker_invalid",
  did_not_run: ["placeholder reported no finding on its canary on src/locales/en.json (de)"],
  score: 100,
  files: [{ path: "src/locales/en.json", findings: [] }],
};

const FAILING: CheckRunResult = {
  pass: false,
  verdict: "failed",
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
          fix: fixFor("blk-2", 7, 14, 0, "use"),
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
          fix: fixFor("blk-2", 19, 28, 0, "overview"),
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
          fix: fixFor("blk-3", 0, 8, 2, "use"),
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
 * A passing run whose voice profile carries configuration to fix. The warnings
 * sit apart from the findings, and the verdict and score are the clean run's.
 */
export const PassingWithWarnings: Story = {
  args: { tabID: "story", result: PASSING_WITH_WARNINGS },
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

/** A run with nothing in scope: it did not run, and never reads as passing. */
export const DidNotRun: Story = {
  args: { tabID: "story", result: NOTHING_TO_CHECK },
};

/** A run whose checker missed its canary: nothing it reported can be trusted. */
export const CheckerInvalid: Story = {
  args: { tabID: "story", result: CHECKER_INVALID },
};
export const CheckerInvalidDark: Story = {
  args: CheckerInvalid.args,
  globals: { theme: "dark" },
};

/** The loading/skeleton state while a run is in flight. */
export const Loading: Story = {
  args: { tabID: "story", forceLoading: true },
};

/**
 * Interactive: Apply fix sends the finding's fix to the change service, and the
 * finding leaves the list once it lands. The in-memory service stands in for
 * the bindings.
 */
export const InteractiveFix: StoryObj<typeof ChecksPanel> = {
  render: () => {
    function Wrapper() {
      const [result, setResult] = useState<CheckRunResult>(FAILING);
      const [changes] = useState<ChangeClient>(() => {
        const memory = new MemoryChanges(CHECKED_BLOCKS);
        return {
          read: (r) => memory.read(r),
          describe: () => memory.describe(),
          history: (r) => memory.history(r),
          apply: async (set) => {
            const res = await memory.apply(set);
            if (res.status === "applied") {
              const fixed = new Set(set.ops.map((op) => ("at" in op ? op.at.block : "")));
              setResult((prev) => {
                const files = prev.files.map((f) => ({
                  ...f,
                  findings: f.findings.filter((x) => !(x.fix && fixed.has(x.block_id ?? ""))),
                }));
                const failing = files.some((f) => f.findings.some((x) => x.fails));
                return { ...prev, pass: !failing, verdict: failing ? "failed" : "passed", files };
              });
            }
            return res;
          },
        };
      });
      return <ChecksPanel tabID="story" result={result} changes={changes} />;
    }
    return <Wrapper />;
  },
};

/**
 * The block changed after the check read it. Apply fix writes nothing, shows
 * the text as it stands and asks before applying the fix to it.
 */
export const StaleFix: Story = {
  name: "Fix: changed since the check",
  args: {
    tabID: "story",
    result: FAILING,
    changes: (() => {
      const memory = new MemoryChanges(CHECKED_BLOCKS);
      memory.touch(
        "src/locales/en.json",
        "blk-2",
        "Please do utilize the dashboard before your credits reset.",
      );
      return memory;
    })(),
  },
};

/**
 * The block changed after the check read it, and its words now have a bold
 * span among them. The fix's plain-text replacement would delete the bold, so
 * after Apply fix the prompt says why and offers only to keep the text.
 */
export const StaleFixOverFormatting: Story = {
  name: "Fix: words now span formatting",
  args: {
    tabID: "story",
    result: FAILING,
    changes: (() => {
      const memory = new MemoryChanges(CHECKED_BLOCKS);
      memory.touch(
        "src/locales/en.json",
        "blk-2",
        'Please util<x id="1"/>ize<x id="/1"/> the dashboard to review your credits.',
      );
      return memory;
    })(),
  },
};
