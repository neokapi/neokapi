import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { fn } from "storybook/test";
import { OpsDiff } from "./OpsDiff";
import type { ChangeSetOp } from "../../types/brand-graph";

const now = "2026-06-13T10:00:00Z";
function mk(seq: number, op: ChangeSetOp["op"], payload: ChangeSetOp["payload"]): ChangeSetOp {
  return {
    workspace_id: "ws-1",
    changeset_id: "cs-1",
    seq,
    op,
    payload,
    base_rev: 4,
    created_by: "sam",
    created_at: now,
  };
}

const ops: ChangeSetOp[] = [
  mk(1, "term.status", {
    concept_id: "c-1",
    locale: "en-US",
    text: "utilize",
    from: "approved",
    to: "forbidden",
  }),
  mk(2, "term.status", {
    concept_id: "c-1",
    locale: "en-US",
    text: "use",
    from: "approved",
    to: "preferred",
  }),
  mk(3, "term.add", {
    concept_id: "c-1",
    term: { text: "Nutzung", locale: "de-DE", status: "proposed" },
  }),
  mk(4, "voice.rule.add", { profile_id: "p-1", list: "forbidden", rule: { term: "utilize" } }),
  mk(5, "relation.add", {
    relation: {
      id: "r-1",
      source_id: "c-1",
      target_id: "c-2",
      relation_type: "REPLACED_BY",
      created_at: now,
    },
  }),
  mk(6, "concept.update", { concept_id: "c-1", definition: "The act of completing a purchase." }),
];

const pad: Decorator = (Story) => (
  <div style={{ maxWidth: 560, padding: 24 }}>
    <Story />
  </div>
);

const meta: Meta<typeof OpsDiff> = {
  title: "Brand Hub/Experiments/OpsDiff",
  component: OpsDiff,
  tags: ["autodocs"],
  decorators: [pad],
};

export default meta;
type Story = StoryObj<typeof OpsDiff>;

/** Read-only diff grouped by category, with governed badges. */
export const Default: Story = { args: { ops } };

/** Editable (a draft): each op can be removed. */
export const Editable: Story = { args: { ops, editable: true, onRemove: fn() } };

export const Empty: Story = { args: { ops: [] } };
