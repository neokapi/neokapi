import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { DecideView } from "../DecideView";
import { DecideScreen } from "../screens";
import type { ProjectScreensPort } from "../port";
import {
  DECIDE_IDS,
  DECIDE_VIEW,
  EMPTY_DECIDE_VIEW,
  LONG_VIEW,
  NAME_VIEW,
  WORD_VIEW,
  MAP_PENDING,
  READER_WITH_FIXES,
} from "./fixtures";

/**
 * Decide: each suggestion waiting for a person as one question. The list on
 * the left holds one row per suggestion; the pane on the right asks the
 * question `kapi rules` asks, with the evidence as its options, what keeping
 * the picked one does, and Keep, Not a rule and Later.
 */
const meta: Meta<typeof DecideView> = {
  title: "Project screens/Rules",
  component: DecideView,
  tags: ["autodocs"],
  args: {
    view: DECIDE_VIEW,
    keyboard: false,
    onAnswer: fn(async () => undefined),
    navigate: fn(),
  },
  decorators: [
    (Story) => (
      <div className="h-[560px] overflow-hidden rounded-lg border border-border bg-background">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof DecideView>;

const dark = { globals: { theme: "dark" } };

/** KapiMart with four questions: a disagreement, a name, a word and a note. */
export const ManyQuestions: Story = {};

export const ManyQuestionsDark: Story = { ...dark, name: "Many questions (dark)" };

/** A name with its forms: which one do your files use, the most used first, and Other…. */
export const NameWithForms: Story = {
  args: { view: NAME_VIEW },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Which one do your files use?")).toBeInTheDocument();
    const options = canvas.getAllByRole("radio");
    await expect(options).toHaveLength(3);
    await userEvent.click(options[1]);
    await expect(canvasElement.querySelector("[data-slot='decide-consequence']")).toHaveTextContent(
      'If you keep "Quick cast"',
    );
  },
};

export const NameWithFormsDark: Story = {
  ...dark,
  name: "Name with forms (dark)",
  args: { view: NAME_VIEW },
};

/** A yes-or-no rule: keep this as a rule, and what keeping it does. */
export const YesOrNoRule: Story = {
  args: { view: WORD_VIEW },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Keep this as a rule?")).toBeInTheDocument();
    await expect(canvas.queryAllByRole("radio")).toHaveLength(0);
  },
};

export const YesOrNoRuleDark: Story = {
  ...dark,
  name: "Yes-or-no rule (dark)",
  args: { view: WORD_VIEW },
};

/** Two assistants disagree: each side with its own uses and what keeping it does. */
export const Disagreement: Story = {
  args: { view: DECIDE_VIEW, defaultSelected: `conflict:${DECIDE_IDS.store}` },
};

/** Nothing waits: the rules count and the way back to the Map. */
export const NothingToDecide: Story = {
  args: { view: EMPTY_DECIDE_VIEW },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Nothing to decide.")).toBeInTheDocument();
    await userEvent.click(canvas.getByRole("button", { name: "Open the Map" }));
    await expect(args.navigate).toHaveBeenCalledWith({ kind: "map" });
  },
};

export const NothingToDecideDark: Story = {
  ...dark,
  name: "Nothing to decide (dark)",
  args: { view: EMPTY_DECIDE_VIEW },
};

/** Forms, a quote and a path that run long. */
export const LongContent: Story = { args: { view: LONG_VIEW } };

/** A port with the reads alone. */
const READ_ONLY: ProjectScreensPort = {
  mapSummary: async () => MAP_PENDING,
  readerFile: async () => READER_WITH_FIXES,
  decideView: async () => DECIDE_VIEW,
};

/** A read-only preview: each question and its evidence, and no Keep, Not a rule or Later. */
export const ReadOnly: StoryObj<typeof DecideScreen> = {
  render: () => (
    <DecideScreen port={READ_ONLY} keyboard={false} navigate={fn()} canNavigate={() => false} />
  ),
};
