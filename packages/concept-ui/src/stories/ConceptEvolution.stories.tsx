import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConceptEvolution } from "../ConceptEvolution";
import { EvolutionGraph } from "../EvolutionGraph";
import { EvolutionRoadmap } from "../EvolutionRoadmap";
import { buildEvolutionModel } from "../evolution-model";
import type { EvolutionInput } from "../evolution-model";
import {
  NOW,
  denseEvolution,
  manyLanguageEvolution,
  richEvolution,
  sparseEvolution,
} from "./evolution.fixtures";

const model = (input: EvolutionInput) => buildEvolutionModel(input, { now: NOW });

function Frame({ width, children }: { width: number | string; children: React.ReactNode }) {
  return (
    <div className="bg-background p-6">
      <div className="rounded-xl border bg-card p-4" style={{ width, maxWidth: "100%" }}>
        {children}
      </div>
    </div>
  );
}

const meta: Meta<typeof ConceptEvolution> = {
  title: "Concept UI/Evolution/ConceptEvolution",
  component: ConceptEvolution,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof ConceptEvolution>;

/**
 * Wide: the horizontal roadmap. The running example — `directory → folder` in
 * en, a Norwegian sibling (`mappe`) branching in, German, and French — with the
 * shared context track (a locale introduced, a governed change-set) above the
 * language lanes, validity bars, and a "now" edge.
 */
export const RoadmapWide: Story = {
  render: () => (
    <Frame width={960}>
      <ConceptEvolution model={model(richEvolution())} />
    </Frame>
  ),
};

/**
 * The SAME model in a narrow container folds to the vertical git-graph — the
 * renderer is chosen from the container's own width, so the panel adapts whether
 * it sits in a wide row or a tight desktop pane. No horizontal scroll.
 */
export const NarrowFold: Story = {
  render: () => (
    <Frame width={360}>
      <ConceptEvolution model={model(richEvolution())} />
    </Frame>
  ),
};

/**
 * Dense history: bursts of routine edits collapse into "N changes" clouds while
 * the milestones that matter — the rename, the change-set, the sibling — stay
 * discrete. Click a cloud (git-graph) to expand it.
 */
export const Dense: Story = {
  render: () => (
    <Frame width={960}>
      <ConceptEvolution model={model(denseEvolution())} />
    </Frame>
  ),
};

/**
 * Many languages: the focused few render in full; the long tail folds into a
 * "+N more languages" band — awareness of the rest without drowning the ones you
 * work in. Expand it to see them all.
 */
export const ManyLanguages: Story = {
  render: () => (
    <Frame width={960}>
      <ConceptEvolution model={model(manyLanguageEvolution())} />
    </Frame>
  ),
};

/** A brand-new concept: just its genesis, no archaeology required. */
export const Sparse: Story = {
  render: () => (
    <Frame width={960}>
      <ConceptEvolution model={model(sparseEvolution())} />
    </Frame>
  ),
};

/** The horizontal roadmap on its own (forced, regardless of width). */
export const RoadmapOnly: Story = {
  render: () => (
    <Frame width="100%">
      <EvolutionRoadmap model={model(richEvolution())} />
    </Frame>
  ),
};

/** The vertical git-graph on its own (forced, regardless of width). */
export const GraphOnly: Story = {
  render: () => (
    <Frame width={460}>
      <EvolutionGraph model={model(denseEvolution())} />
    </Frame>
  ),
};
