import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ReaderView } from "../ReaderView";
import { ReaderScreen } from "../screens";
import type { ProjectScreensPort } from "../port";
import {
  DECIDE_VIEW,
  MAP_PENDING,
  READER_CLEAN,
  READER_REFUSED,
  READER_WITH_FIXES,
  READER_WITHOUT_FIXES,
} from "./fixtures";

/**
 * The Reader: one file's blocks with each finding on the words it is about,
 * and the rules that hold at the file down the margin. A finding with a fix
 * carries Fix; the header's button applies every fix in the file.
 */
const meta: Meta<typeof ReaderView> = {
  title: "Project screens/Reader",
  component: ReaderView,
  tags: ["autodocs"],
  args: {
    reader: READER_WITH_FIXES,
    onFix: fn(),
    onFixAll: fn(),
    onBack: fn(),
    backLabel: "Back to the Map",
    navigate: fn(),
  },
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof ReaderView>;

/** Two term rules with a replacement, each with Fix, and a voice rule edited by hand. */
export const FindingsWithFixes: Story = {};

/** A voice rule and a placeholder a translation lost: neither has a fix. */
export const FindingsWithoutFixes: Story = { args: { reader: READER_WITHOUT_FIXES } };

/** No findings: the file and the rules that hold there. */
export const NoFindings: Story = { args: { reader: READER_CLEAN } };

/** A fix the change service refused, with its reason under the finding. */
export const FixRefused: Story = { args: { reader: READER_REFUSED } };

/** Opened from a finding on the Checks page, at its block. */
export const AtABlock: Story = { args: { focusBlockID: "p3", backLabel: "Back to checks" } };

/** A port with the reads alone. */
const READ_ONLY: ProjectScreensPort = {
  mapSummary: async () => MAP_PENDING,
  readerFile: async () => READER_WITH_FIXES,
  decideView: async () => DECIDE_VIEW,
};

/** A read-only preview: the findings, and no Fix, through a port with no actions. */
export const ReadOnly: StoryObj<typeof ReaderScreen> = {
  render: () => (
    <ReaderScreen
      port={READ_ONLY}
      path="docs/billing.md"
      navigate={fn()}
      canNavigate={() => false}
    />
  ),
};
