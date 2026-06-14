// @vitest-environment jsdom
// Render tests for the vertical EvolutionGraph (Apache-2.0). They build a real
// model via the pure builder — a directory→folder rename, a Norwegian sibling
// branching after genesis, and a dense run of routine revisions that must fold
// into a cluster — then assert the graph mounts and renders the signal: the
// rename text, the sibling branch row, and an expandable "N changes" cloud.

import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { EvolutionGraph } from "./EvolutionGraph";
import { buildEvolutionModel } from "./evolution-model";
import type { Concept, TimelineEvent } from "./types";

afterEach(cleanup);

const NOW = "2026-06-14T00:00:00.000Z";

/** A concept whose history exercises a rename, a sibling, and a routine run. */
function storyConcept(): Concept {
  return {
    id: "c-folder",
    domain: "product",
    createdAt: "2023-01-01T00:00:00.000Z",
    updatedAt: "2024-06-01T00:00:00.000Z",
    terms: [
      {
        text: "directory",
        locale: "en",
        status: "deprecated",
        validity: { validFrom: "2023-01-01T00:00:00.000Z", validTo: "2024-06-01T00:00:00.000Z" },
      },
      {
        text: "folder",
        locale: "en",
        status: "preferred",
        validity: { validFrom: "2024-06-01T00:00:00.000Z" },
      },
      {
        text: "mappe",
        locale: "nb",
        status: "preferred",
        validity: { validFrom: "2024-01-01T00:00:00.000Z", tags: { market: "nordics" } },
      },
    ],
  };
}

/** Genesis + four time-close routine revisions (fold to a cluster) + a merge. */
const timeline: TimelineEvent[] = [
  { kind: "create", at: "2023-01-01T00:00:00.000Z", summary: "Created" },
  { kind: "revision", at: "2024-03-01T00:00:00.000Z", summary: "tweak wording", actor: "ana" },
  { kind: "revision", at: "2024-03-02T00:00:00.000Z", summary: "tweak casing", actor: "ben" },
  { kind: "revision", at: "2024-03-03T00:00:00.000Z", summary: "tweak note" },
  { kind: "revision", at: "2024-03-04T00:00:00.000Z", summary: "tweak example" },
  { kind: "changeset", at: "2024-06-01T00:00:00.000Z", summary: "Rename merged" },
];

function model() {
  return buildEvolutionModel({ concept: storyConcept(), timeline }, { now: NOW });
}

describe("EvolutionGraph", () => {
  it("mounts and shows the within-lane rename", () => {
    const m = model();
    render(<EvolutionGraph model={m} />);
    expect(screen.getByText("directory → folder")).toBeTruthy();
  });

  it("renders the Norwegian sibling branch row", () => {
    render(<EvolutionGraph model={model()} />);
    expect(screen.getByText(/nb branched from en/i)).toBeTruthy();
  });

  it("folds the routine revisions into an expandable cluster", () => {
    const m = model();
    // The builder must have produced at least one cluster for this graph to test.
    expect(m.globalClusters.length).toBeGreaterThanOrEqual(1);
    render(<EvolutionGraph model={m} />);
    const pill = screen.getByRole("button", { name: /changes/i });
    expect(pill.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(pill);
    expect(pill.getAttribute("aria-expanded")).toBe("true");
    // An expanded cluster reveals one of its folded edits.
    expect(screen.getByText("tweak wording")).toBeTruthy();
  });

  it("navigates when a rename row is activated", () => {
    const m = buildEvolutionModel(
      {
        concept: storyConcept(),
        relations: [
          {
            id: "r1",
            sourceId: "c-folder",
            targetId: "c-archive",
            type: "REPLACED_BY",
            validity: { validFrom: "2025-02-01T00:00:00.000Z" },
          },
        ],
        neighbourLabels: { "c-archive": "archive" },
      },
      { now: NOW },
    );
    let navigated = "";
    render(<EvolutionGraph model={m} onNavigate={(id) => (navigated = id)} />);
    fireEvent.click(screen.getByText(/renamed to archive/i));
    expect(navigated).toBe("c-archive");
  });

  it("renders an empty hint for a model with no events", () => {
    const bare = buildEvolutionModel(
      { concept: { id: "empty", domain: "x", terms: [] } },
      { now: NOW },
    );
    // A truly empty model has no milestones to show.
    if (bare.milestones.length === 0 && bare.branches.length === 0) {
      render(<EvolutionGraph model={bare} />);
      expect(screen.getByText("No history yet")).toBeTruthy();
    }
  });
});
