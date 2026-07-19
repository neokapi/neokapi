import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProposeSourceChangeDialog } from "../../components/review/ProposeSourceChangeDialog";
import { withProviders } from "../decorators";

const meta: Meta<typeof ProposeSourceChangeDialog> = {
  title: "Review/ProposeSourceChangeDialog",
  component: ProposeSourceChangeDialog,
  decorators: [withProviders],
  args: {
    open: true,
    onOpenChange: fn(),
    projectId: "proj-1",
    blockId: "b1",
    itemName: "ui.json",
    sourceLocale: "en-US",
    foundInLocale: "fr-FR",
    initialSource: "Colour picker",
    localeCount: 3,
    onDone: fn(),
  },
};
export default meta;
type Story = StoryObj<typeof ProposeSourceChangeDialog>;

/** The back-to-source lane: propose a source fix while reviewing a target. */
export const Default: Story = {};

/** Proposing for a project with a single target locale. */
export const SingleLocale: Story = {
  args: { localeCount: 1 },
};
