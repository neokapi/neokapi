import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WhatsNew, WorkspaceNumbers } from "../components/WhatsNew";
import {
  FIELD_GUIDE,
  KAPIMART,
  NOTHING,
  ONE_WORKING,
  SEVERAL,
  standing,
} from "./fixtures/standings";
import type { ContextProjectStanding } from "../types/api";

/** The counts above the rows, as the home composes the two. */
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
      <div className="mx-auto max-w-4xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof HomeTop>;

const dark = { theme: "dark" };

/**
 * One project with four new items while claude-code is working in it: its
 * mark, a working chip with a live dot, and a chip for each kind it noticed.
 */
export const OneProjectWorking: Story = { args: { standings: ONE_WORKING } };
export const OneProjectWorkingDark: Story = { args: OneProjectWorking.args, globals: dark };

/**
 * Several projects, newest session first. KapiMart has a disagreement, so it
 * carries the warning chip and reads Decide, and "needs you" is the one count
 * in the warning ink.
 */
export const SeveralProjects: Story = { args: { standings: SEVERAL } };
export const SeveralProjectsDark: Story = { args: SeveralProjects.args, globals: dark };

/**
 * A session that noticed one of every kind, and a project whose latest session
 * said nothing about kinds, so its chip counts its suggestions.
 */
export const EveryKind: Story = {
  args: {
    standings: [
      {
        ...KAPIMART,
        latest: {
          ...KAPIMART.latest!,
          new_by_theme: { names: 2, words: 1, writing: 1, memory: 3 },
        },
        conflicts: 2,
      },
      standing({
        project: "prj_quiet",
        project_name: "Release Notes",
        rules: 4,
        suggested: 6,
        latest: {
          actor: { kind: "person", name: "Ingrid Solberg", session: "day-3" },
          at: new Date(Date.now() - 50 * 3_600_000).toISOString(),
          working: false,
          recorded: 6,
          corrected: 0,
        },
      }),
    ],
  },
};
export const EveryKindDark: Story = { args: EveryKind.args, globals: dark };

/** Projects hold rules and nothing is new: the counts stay, and What's new says so in one line. */
export const NothingNew: Story = { args: { standings: [FIELD_GUIDE] } };

/** Nothing is recorded in any project yet: zeros, and one line saying what will appear. */
export const NothingRecorded: Story = { args: { standings: NOTHING } };
