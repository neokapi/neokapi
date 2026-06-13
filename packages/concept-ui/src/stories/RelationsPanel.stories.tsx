import { useMemo, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { RelationsPanel } from "../RelationsPanel";
import type { ConceptDataSource } from "../adapter";
import { resolveCapabilities } from "../adapter";
import type { Concept, Relation, TermStatus } from "../types";
import { useResource } from "../useResource";
import { makeMemorySource } from "./fixtures";

// A harness that loads the centred concept and re-centres on relation navigation,
// so the collapse + navigate behaviour is exercised the way a host wires it.
function RelationsHarness({
  source,
  start = "checkout",
  collapseThreshold,
}: {
  source: ConceptDataSource;
  start?: string;
  collapseThreshold?: number;
}) {
  const [id, setId] = useState(start);
  const caps = useMemo(() => resolveCapabilities(source), [source]);
  const { data: concept } = useResource(() => source.getConcept(id), [source, id]);
  return (
    <div className="mx-auto max-w-md p-6">
      {concept && (
        <RelationsPanel
          concept={concept}
          source={source}
          capabilities={caps}
          onNavigate={setId}
          collapseThreshold={collapseThreshold}
        />
      )}
    </div>
  );
}

// A bespoke source whose hub has many related concepts, so one lane collapses to
// a single "N related →" affordance while a small lane stays inline.
function makeHubSource(): ConceptDataSource {
  const status: TermStatus = "approved";
  const neighbours: Concept[] = Array.from({ length: 9 }, (_, i) => ({
    id: `n${i}`,
    domain: "commerce",
    terms: [{ text: `Adjacent concept ${i + 1}`, locale: "en-US", status }],
  }));
  const hub: Concept = {
    id: "hub",
    domain: "commerce",
    definition: "A concept wired to many neighbours, to show a lane collapsing.",
    terms: [{ text: "Checkout flow", locale: "en-US", status: "preferred" }],
  };
  const concepts = [hub, ...neighbours];
  const relations: Relation[] = neighbours.map((n, i) => ({
    id: `hr${i}`,
    sourceId: "hub",
    targetId: n.id,
    type: "RELATED",
  }));
  relations.push({ id: "hu", sourceId: "hub", targetId: "n0", type: "USE_INSTEAD" });
  const find = (id: string) => concepts.find((c) => c.id === id) ?? null;
  return {
    listConcepts: () => ({ concepts, total: concepts.length }),
    getConcept: (id) => find(id),
    getRelations: (cid) => relations.filter((r) => r.sourceId === cid || r.targetId === cid),
    getConceptSummary: (id) => find(id),
  };
}

const richSource = makeMemorySource();
const coreSource = makeMemorySource({ rich: false, editable: false });
const hubSource = makeHubSource();
const failingSource: ConceptDataSource = {
  ...makeMemorySource(),
  getRelations: () => Promise.reject(new Error("Server unavailable (503)")),
};

const meta: Meta<typeof RelationsPanel> = {
  title: "Concept UI/RelationsPanel",
  component: RelationsPanel,
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof RelationsPanel>;

/** The editable platform path: grouped lanes, plus add (Relate) and inline remove. */
export const Editable: Story = {
  render: () => <RelationsHarness source={richSource} />,
};

/** Framework-only (local termbase): the same lanes, read-only — no edit affordances. */
export const ReadOnly: Story = {
  render: () => <RelationsHarness source={coreSource} />,
};

/** A lane with many neighbours collapses to "9 related →"; click to expand. */
export const CollapsingGroup: Story = {
  render: () => <RelationsHarness source={hubSource} start="hub" />,
};

/** A failed relations read surfaces an error rather than reading as "no relations". */
export const FetchError: Story = {
  render: () => <RelationsHarness source={failingSource} />,
};
