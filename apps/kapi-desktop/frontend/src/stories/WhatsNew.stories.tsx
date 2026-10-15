import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WhatsNew, WorkspaceNumbers } from "../components/WhatsNew";
import { FIELD_GUIDE, NOTHING, ONE_WORKING, SEVERAL } from "./fixtures/standings";
import type { ContextProjectStanding } from "../types/api";

/** The numbers sentence above the rows, as the home composes the two. */
function HomeTop({
  standings,
  onOpen,
}: {
  standings: ContextProjectStanding[] | null;
  onOpen: (key: string) => void;
}) {
  return (
    <div className="space-y-8">
      <WorkspaceNumbers standings={standings} />
      <WhatsNew standings={standings} onOpen={onOpen} />
    </div>
  );
}

const meta: Meta<typeof HomeTop> = {
  title: "Components/WhatsNew",
  component: HomeTop,
  tags: ["autodocs"],
  args: { onOpen: fn() },
  decorators: [
    (Story) => (
      <div className="mx-auto max-w-3xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof HomeTop>;

/** One project with four new items while claude-code is working in it; the dot pulses. */
export const OneProjectWorking: Story = { args: { standings: ONE_WORKING } };

/** Several projects, newest session first; KapiMart has a disagreement, so it reads Decide. */
export const SeveralProjects: Story = { args: { standings: SEVERAL } };

/** Projects hold rules and nothing is new: the numbers stay, the rows go. */
export const NothingNew: Story = { args: { standings: [FIELD_GUIDE] } };

/** Nothing is recorded in any project yet. */
export const NothingRecorded: Story = { args: { standings: NOTHING } };
