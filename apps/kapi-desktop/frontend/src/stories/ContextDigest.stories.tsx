import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ContextDigestView } from "../components/ContextDigest";
import {
  CONTEXT_DIGEST,
  EMPTY_DIGEST,
  LAST_LOOKED,
  QUIET_DIGEST,
  onlySection,
} from "./fixtures/contextDigest";

const meta: Meta<typeof ContextDigestView> = {
  title: "Components/ContextDigest",
  component: ContextDigestView,
  tags: ["autodocs"],
  args: {
    digest: CONTEXT_DIGEST,
    since: LAST_LOOKED,
    keyboard: false,
    onKeep: fn(),
    onKeepGroup: fn(),
    onDrop: fn(),
    onChoose: fn(),
    onRevert: fn(),
    onOpenFile: fn(),
  },
  decorators: [
    (Story) => (
      <div className="mx-auto max-w-3xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ContextDigestView>;

/** Every section: a conflict, rules established and how, suggestions by theme, drift, numbers. */
export const Default: Story = {};

/** Needs you: two agents disagree about "login", and the person chooses one. */
export const Conflicts: Story = { args: { digest: onlySection("conflicts") } };

/** Rules in force since the person last looked, with the evidence they rest on. */
export const Established: Story = { args: { digest: onlySection("established") } };

/** Suggestions grouped by theme, then by collection, with a keep-all per group. */
export const Suggested: Story = { args: { digest: onlySection("suggested") } };

/** Content moving away from an established rule. */
export const Drift: Story = { args: { digest: onlySection("drift") } };

/** Nothing new since the last look: the numbers, and what the person already saw under Earlier. */
export const NothingNew: Story = { args: { digest: QUIET_DIGEST } };

/** A project nobody has looked at and nothing is recorded about. */
export const Empty: Story = { args: { digest: EMPTY_DIGEST, since: undefined } };

/** No checkout of the project on this machine: the digest reads, and offers no decisions. */
export const ReadOnly: Story = { args: { canDecide: false, onOpenFile: undefined } };

/** The keyboard session: j and k move, a keeps, c changes, d drops, g keeps the group. */
export const Keyboard: Story = { args: { keyboard: true } };
