import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { ReachPanel } from "./ReachPanel";
import type { ChangeSetImpact, Reach } from "../../types/brand-graph";

const base: ChangeSetImpact = {
  total_blocks: 1280,
  affected_blocks: 34,
  new_violations: 12,
  resolved: 7,
  words: 210,
  projects: null,
  samples: null,
};

const withReach = (reach: Reach, over: Partial<ChangeSetImpact> = {}): ChangeSetImpact => ({
  ...base,
  affected_blocks: reach.annotate.blocks + reach.transform.blocks,
  reach,
  ...over,
});

const mostlyAnnotate: Reach = {
  annotate: {
    blocks: 28,
    words: 170,
    collections: 3,
    projects: 2,
    targets: 54,
    approved: 21,
    locales: ["de", "fr", "nb"],
  },
  transform: {
    blocks: 6,
    words: 40,
    collections: 1,
    projects: 1,
    targets: 12,
    approved: 9,
    locales: ["de", "nb"],
  },
  transform_projects: [{ project_id: "p-web", project_name: "Marketing Website" }],
};

const meta: Meta<typeof ReachPanel> = {
  title: "Brand Hub/Experiments/ReachPanel",
  component: ReachPanel,
  tags: ["autodocs"],
  decorators: [
    ((Story) => (
      <div style={{ maxWidth: 640, padding: 24 }}>
        <Story />
      </div>
    )) as Decorator,
  ],
};

export default meta;
type Story = StoryObj<typeof ReachPanel>;

/** The common shape: a draft that mostly re-flags, with a little rewriting in it. */
export const Default: Story = { args: { impact: withReach(mostlyAnnotate) } };

/** Nothing prescribes a rewrite: the whole draft is an annotation. */
export const AnnotateOnly: Story = {
  args: {
    impact: withReach({
      annotate: mostlyAnnotate.annotate,
      transform: {
        blocks: 0,
        words: 0,
        collections: 0,
        projects: 0,
        targets: 0,
        approved: 0,
        locales: [],
      },
      transform_projects: [],
    }),
  },
};

/** Every affected block names a successor — the expensive shape. */
export const TransformHeavy: Story = {
  args: {
    impact: withReach({
      annotate: {
        blocks: 2,
        words: 12,
        collections: 1,
        projects: 1,
        targets: 4,
        approved: 0,
        locales: ["nb"],
      },
      transform: {
        blocks: 140,
        words: 2100,
        collections: 6,
        projects: 3,
        targets: 380,
        approved: 290,
        locales: ["de", "fr", "ja", "nb"],
      },
      transform_projects: [
        { project_id: "p-web", project_name: "Marketing Website" },
        { project_id: "p-app", project_name: "Product App" },
        { project_id: "p-docs", project_name: "Docs" },
      ],
    }),
  },
};

/** Nothing translated yet: the re-check has nothing to pull back. */
export const NothingTranslated: Story = {
  args: {
    impact: withReach({
      annotate: {
        blocks: 9,
        words: 60,
        collections: 2,
        projects: 1,
        targets: 0,
        approved: 0,
        locales: [],
      },
      transform: {
        blocks: 0,
        words: 0,
        collections: 0,
        projects: 0,
        targets: 0,
        approved: 0,
        locales: [],
      },
      transform_projects: [],
    }),
  },
};

/** The stored summary: the counts stand, the locale lists were never kept. */
export const FromTheStoredSummary: Story = {
  args: {
    impact: withReach(
      {
        annotate: {
          blocks: 28,
          words: 0,
          collections: 0,
          projects: 0,
          targets: 54,
          approved: 21,
          locales: [],
        },
        transform: {
          blocks: 6,
          words: 0,
          collections: 0,
          projects: 1,
          targets: 12,
          approved: 9,
          locales: [],
        },
        transform_projects: [],
      },
      { stored: true, computed_at: "2026-08-14T09:12:00Z" },
    ),
  },
};

/** Nothing affected: the panel renders nothing rather than an empty bar. */
export const Absent: Story = { args: { impact: base } };
