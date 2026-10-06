import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ChangeResult } from "@neokapi/contract-types";

import { KeptConflicts } from "../components/KeptConflicts";
import type { ChangeClient } from "../lib/changes";
import { editConflict, fileConflict } from "./fixtures/keptConflicts";

/** A change service that applies every change set. */
const applies: ChangeClient = {
  read: async () => null,
  apply: async (set) =>
    ({
      schema: "kapi.change-result/v1",
      status: "applied",
      ops: set.ops.map((op, i) => ({ i, op: op.op, status: "applied" })),
      docs: [],
    }) as unknown as ChangeResult,
  describe: async () => null,
  history: async () => null,
};

/** A change service that finds the held wording moved since the read. */
const stale: ChangeClient = {
  ...applies,
  apply: async (set) =>
    ({
      schema: "kapi.change-result/v1",
      status: "refused",
      ops: set.ops.map((op, i) => ({
        i,
        op: op.op,
        status: "refused",
        error: { code: "stale", message: "edition nl of block title moved" },
        current: { rev: "r:5555555555555555", text: "Tij venster" },
      })),
      docs: [],
    }) as unknown as ChangeResult,
};

const meta: Meta<typeof KeptConflicts> = {
  title: "Pages/Kept Conflicts",
  component: KeptConflicts,
  parameters: { layout: "padded" },
  args: { tabID: "t1", client: applies, release: async () => {} },
};

export default meta;
type Story = StoryObj<typeof KeptConflicts>;

/** Two machines edited one Dutch draft from one version; one edit did not land. */
export const EditThatDidNotLand: Story = { args: { conflicts: [editConflict] } };

/**
 * The French file appeared without the wording a person kept in the
 * workspace; one block the file does not hold at all.
 */
export const WordingTheFileDoesNotHold: Story = { args: { conflicts: [fileConflict] } };

/** Both kinds at once, as a project shows them. */
export const BothKinds: Story = { args: { conflicts: [editConflict, fileConflict] } };

/** Deciding finds the wording moved since the read: nothing is written. */
export const StaleDecision: Story = { args: { conflicts: [editConflict], client: stale } };

/** No conflicts: nothing is drawn. */
export const NoConflicts: Story = { args: { conflicts: [] } };
