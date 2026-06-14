import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { Decorator } from "@storybook/react";
import { ExperimentDetailView } from "./ExperimentDetailView";
import { brandHubOverrides } from "../../stories/brandHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";
import type { ChangeSetDetail, ChangeSetImpact, ChangeSetOp } from "../../types/brand-graph";
import type { User } from "../../types/api";

const now = "2026-06-13T10:00:00Z";
const earlier = "2026-06-01T09:00:00Z";

const ops: ChangeSetOp[] = [
  {
    workspace_id: "ws-1",
    changeset_id: "cs-1",
    seq: 1,
    op: "term.status",
    payload: {
      concept_id: "c-checkout",
      locale: "en-US",
      text: "utilize",
      from: "approved",
      to: "forbidden",
    },
    base_rev: 4,
    created_by: "sam",
    created_at: now,
  },
  {
    workspace_id: "ws-1",
    changeset_id: "cs-1",
    seq: 2,
    op: "term.status",
    payload: {
      concept_id: "c-checkout",
      locale: "en-US",
      text: "use",
      from: "approved",
      to: "preferred",
    },
    base_rev: 4,
    created_by: "sam",
    created_at: now,
  },
  {
    workspace_id: "ws-1",
    changeset_id: "cs-1",
    seq: 3,
    op: "relation.add",
    payload: {
      relation: {
        id: "r-new",
        source_id: "c-checkout",
        target_id: "c-pay",
        relation_type: "USE_INSTEAD",
        created_at: now,
      },
    },
    base_rev: 4,
    created_by: "sam",
    created_at: now,
  },
];

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
      collections: [
        {
          collection_id: "col-1",
          collection_name: "Pages",
          affected_blocks: 22,
          new_violations: 8,
          resolved: 5,
          words: 140,
          locales: [
            {
              stream: "main",
              locale: "de-DE",
              affected_blocks: 14,
              new_violations: 5,
              resolved: 3,
              words: 90,
            },
            {
              stream: "main",
              locale: "fr-FR",
              affected_blocks: 8,
              new_violations: 3,
              resolved: 2,
              words: 50,
            },
          ],
        },
      ],
    },
    {
      project_id: "p-app",
      project_name: "Mobile App",
      affected_blocks: 12,
      new_violations: 4,
      resolved: 2,
      words: 70,
      collections: [
        {
          collection_id: "col-2",
          collection_name: "Strings",
          affected_blocks: 12,
          new_violations: 4,
          resolved: 2,
          words: 70,
          locales: [
            {
              stream: "main",
              locale: "de-DE",
              affected_blocks: 7,
              new_violations: 3,
              resolved: 1,
              words: 40,
            },
            {
              stream: "main",
              locale: "fr-FR",
              affected_blocks: 5,
              new_violations: 1,
              resolved: 1,
              words: 30,
            },
          ],
        },
      ],
    },
  ],
  samples: [
    {
      project_id: "p-web",
      stream: "main",
      collection_id: "col-1",
      collection_name: "Pages",
      locale: "de-DE",
      item_name: "pricing.de.json",
      block_id: "b-1",
      text: "Wir utilize modernste Technologie für Ihren Checkout.",
      new_violations: 1,
    },
    {
      project_id: "p-app",
      stream: "main",
      collection_id: "col-2",
      collection_name: "Strings",
      locale: "fr-FR",
      item_name: "settings.fr.json",
      block_id: "b-2",
      text: "Utilisez le paiement rapide à la caisse.",
      resolved: 1,
    },
  ],
};

const base: ChangeSetDetail = {
  id: "cs-1",
  workspace_id: "ws-1",
  name: "Retire ‘utilize’ in favour of ‘use’",
  description: "Ban the verbose verb, promote the plain one, and link the concepts.",
  status: "in_review",
  created_by: "sam",
  created_at: earlier,
  updated_at: now,
  submitted_at: now,
  governed: true,
  ops,
  reviews: [],
  pilots: [
    {
      workspace_id: "ws-1",
      changeset_id: "cs-1",
      project_id: "p-web",
      stream: "main",
      created_by: "sam",
      created_at: now,
    },
  ],
};

function user(id: string): User {
  return { id, email: `${id}@demo.test`, name: id, avatar_url: "" };
}

function decoratorFor(detail: ChangeSetDetail, currentUserId = "reviewer"): Decorator {
  return createProvidersDecorator(undefined, {
    ...brandHubOverrides,
    getChangeset: async () => detail,
    getChangesetBlastRadius: async () => impact,
    getCurrentUser: async () => user(currentUserId),
  });
}

const pad: Decorator = (Story) => (
  <div style={{ padding: 24 }}>
    <Story />
  </div>
);

const meta: Meta<typeof ExperimentDetailView> = {
  title: "Brand Hub/Experiments/ExperimentDetailView",
  component: ExperimentDetailView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { changesetId: "cs-1", onBack: fn() },
};

export default meta;
type Story = StoryObj<typeof ExperimentDetailView>;

/** A draft: ops are editable, the blast radius is measured, ready to submit. */
export const Draft: Story = {
  decorators: [decoratorFor({ ...base, status: "draft", reviews: [], pilots: [] }), pad],
};

/** In review, awaiting a verdict from someone other than the author. */
export const InReview: Story = {
  decorators: [decoratorFor({ ...base, status: "in_review" })],
};

/** The author viewing their own in-review change-set: approval is blocked (SoD). */
export const InReviewAsAuthor: Story = {
  decorators: [decoratorFor({ ...base, status: "in_review" }, "sam")],
};

/** Approved with a second person's verdict — ready to merge. */
export const Approved: Story = {
  decorators: [
    decoratorFor({
      ...base,
      status: "approved",
      reviews: [
        {
          workspace_id: "ws-1",
          changeset_id: "cs-1",
          reviewer: "alex",
          verdict: "approve",
          comment: "Matches the brand book.",
          created_at: now,
        },
      ],
    }),
  ],
};

/** Merged — terminal, applied to the live graph. */
export const Merged: Story = {
  decorators: [
    decoratorFor({
      ...base,
      status: "merged",
      merged_at: now,
      merged_by: "alex",
      pilots: [],
      reviews: [
        {
          workspace_id: "ws-1",
          changeset_id: "cs-1",
          reviewer: "alex",
          verdict: "approve",
          created_at: now,
        },
      ],
    }),
  ],
};
