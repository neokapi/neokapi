import type { Meta, StoryObj } from "@storybook/react-vite";
import { FindingSnippet } from "../../components/review";
import type { Run } from "../../components/preview/types";

const plain: Run[] = [{ text: "Please utilize the dashboard to review your credits." }];

const withCodes: Run[] = [
  { text: "Your credits reset on " },
  { ph: { id: "date", type: "var", data: "{date}", equiv: "date" } },
  { text: ". See the " },
  { pcOpen: { id: "b", type: "bold", data: "<b>", equiv: "b" } },
  { text: "billing page" },
  { pcClose: { id: "b", type: "bold", data: "</b>", equiv: "b" } },
  { text: " for details." },
];

const arabic: Run[] = [{ text: "تم تغيير أكمي كلاود في الترجمة" }];

const meta: Meta<typeof FindingSnippet> = {
  title: "Review/FindingSnippet",
  component: FindingSnippet,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "A finding in the text it was raised on: the block's runs read as the document reads them, with the finding's span marked in its tone and the inline codes drawn as chips. The desktop's Checks page and the platform's problems panel both read a finding through this.",
      },
    },
  },
  render: (args) => (
    <div className="w-[28rem] rounded-md border bg-card p-3 text-xs text-card-foreground">
      <FindingSnippet {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof FindingSnippet>;

/** A range anchor over the offending words. */
export const RangeAnchor: Story = {
  args: {
    runs: plain,
    locale: "en",
    anchor: { kind: "range", start: { run: 0, offset: 7 }, end: { run: 0, offset: 14 } },
    tone: "destructive",
    label: 'Forbidden term "utilize" found',
  },
};
export const RangeAnchorDark: Story = { args: RangeAnchor.args, globals: { theme: "dark" } };

/** A block anchor: the checker objects to the block as a whole. */
export const BlockAnchor: Story = {
  args: {
    runs: plain,
    locale: "en",
    anchor: { kind: "block" },
    tone: "warning",
    label: "Tone reads more formal than the brand's casual register",
  },
};
export const BlockAnchorDark: Story = { args: BlockAnchor.args, globals: { theme: "dark" } };

/**
 * A source-side finding on a block with inline codes: the placeholder and the
 * bold pair stay in the reading as chips, and the marked words sit between them.
 */
export const SourceWithCodes: Story = {
  args: {
    runs: withCodes,
    locale: "en",
    anchor: { kind: "range", start: { run: 4, offset: 0 }, end: { run: 5 } },
    tone: "warning",
    label: 'Say "billing overview" rather than "billing page"',
  },
};
export const SourceWithCodesDark: Story = {
  args: SourceWithCodes.args,
  globals: { theme: "dark" },
};

/** A run anchor names a placeholder, so the mark goes around its chip. */
export const RunAnchorOnPlaceholder: Story = {
  args: {
    runs: withCodes,
    locale: "en",
    anchor: { kind: "run", runId: "date" },
    tone: "destructive",
    label: "The de target drops {date}.",
  },
};

/** A right-to-left block reads in its own direction. */
export const RightToLeft: Story = {
  args: {
    runs: arabic,
    locale: "ar-EG",
    anchor: { kind: "range", start: { run: 0, offset: 10 }, end: { run: 0, offset: 20 } },
    tone: "destructive",
    label: 'Do-not-translate term "Acme Cloud" is missing from the ar target',
  },
};

/** The checker quoted the text and the block's runs are unavailable. */
export const FallbackText: Story = {
  args: {
    fallbackText: "{count}",
    locale: "de-DE",
    tone: "destructive",
    label: "Placeholder {count} is missing from the de target",
  },
};
export const FallbackTextDark: Story = { args: FallbackText.args, globals: { theme: "dark" } };
