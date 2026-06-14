import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { fn } from "storybook/test";
import { Button } from "@neokapi/ui-primitives";
import { MergeConfirmDialog } from "./MergeConfirmDialog";
import { brandHubOverrides } from "../../stories/brandHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";
import type { ChangeSetImpact, MergeResult } from "../../types/brand-graph";

const impact: ChangeSetImpact = {
  total_blocks: 1280,
  affected_blocks: 34,
  new_violations: 12,
  resolved: 7,
  words: 210,
  projects: [
    {
      project_id: "p-web",
      project_name: "Marketing Website",
      affected_blocks: 22,
      new_violations: 8,
      resolved: 5,
      words: 140,
      collections: [],
    },
  ],
  samples: [],
};

function Harness() {
  const [open, setOpen] = useState(true);
  return (
    <div style={{ padding: 24 }}>
      <Button onClick={() => setOpen(true)}>Merge experiment</Button>
      <MergeConfirmDialog
        open={open}
        onOpenChange={setOpen}
        changesetId="cs-1"
        changesetName="Retire ‘utilize’"
        onMerged={fn()}
      />
    </div>
  );
}

const meta: Meta<typeof MergeConfirmDialog> = {
  title: "Brand Hub/Experiments/MergeConfirmDialog",
  component: MergeConfirmDialog,
  parameters: { layout: "centered" },
};

export default meta;
type Story = StoryObj<typeof MergeConfirmDialog>;

const cleanMerge: MergeResult = {
  changeset_id: "cs-1",
  revisions_created: 2,
  pilots_stopped: 1,
  applied_ops: [1, 2],
};

/** Re-shows the blast radius; merging succeeds. */
export const Default: Story = {
  render: () => <Harness />,
  decorators: [
    createProvidersDecorator(undefined, {
      ...brandHubOverrides,
      getChangesetBlastRadius: async () => impact,
      mergeChangeset: async () => cleanMerge,
    }) as Decorator,
  ],
};

/** A stale-draft conflict (409) is surfaced clearly with re-base guidance. */
export const Conflict: Story = {
  render: () => <Harness />,
  decorators: [
    createProvidersDecorator(undefined, {
      ...brandHubOverrides,
      getChangesetBlastRadius: async () => impact,
      mergeChangeset: async () => {
        throw new Error(
          `409: ${JSON.stringify({
            error: "change-set has stale-draft conflicts",
            conflicts: [
              {
                seq: 2,
                concept_id: "c-checkout",
                reason: "op authored against revision 4 but concept is at revision 5",
              },
            ],
          })}`,
        );
      },
    }) as Decorator,
  ],
};
