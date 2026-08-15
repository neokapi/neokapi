import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import { ReachPanel } from "../brand-hub/experiments/ReachPanel";
import type { ChangeSetImpact, Reach, ReachClass } from "../types/brand-graph";

const cls = (over: Partial<ReachClass> = {}): ReachClass => ({
  blocks: 0,
  words: 0,
  collections: 0,
  projects: 0,
  targets: 0,
  approved: 0,
  locales: [],
  ...over,
});

const impactWith = (reach: Reach, over: Partial<ChangeSetImpact> = {}): ChangeSetImpact => ({
  total_blocks: 1000,
  affected_blocks: reach.annotate.blocks + reach.transform.blocks,
  new_violations: 0,
  resolved: 0,
  words: 0,
  projects: null,
  samples: null,
  reach,
  ...over,
});

describe("ReachPanel", () => {
  // A report computed before the split existed carries none, and the panel is
  // absent rather than rendering zeros that read as "this costs nothing".
  it("renders nothing when the report carries no split", () => {
    const { container } = render(
      <ReachPanel
        impact={{
          total_blocks: 1000,
          affected_blocks: 12,
          new_violations: 12,
          resolved: 0,
          words: 90,
          projects: null,
          samples: null,
        }}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when the split covers no content", () => {
    const { container } = render(
      <ReachPanel
        impact={impactWith({ annotate: cls(), transform: cls(), transform_projects: [] })}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("prices an annotation as a re-check that pulls translations back", () => {
    render(
      <ReachPanel
        impact={impactWith({
          annotate: cls({
            blocks: 12,
            collections: 3,
            projects: 1,
            targets: 20,
            approved: 8,
            locales: ["de", "nb"],
          }),
          transform: cls(),
          transform_projects: [],
        })}
      />,
    );

    expect(screen.getByRole("heading", { name: "What this asks for" })).toBeInTheDocument();
    expect(screen.getByText("Re-check fans out")).toBeInTheDocument();
    expect(screen.getByText(/20 translations return to review in de, nb/)).toBeInTheDocument();
    expect(screen.getByText(/8 already approved/)).toBeInTheDocument();
    expect(screen.queryByText("Source gets rewritten")).not.toBeInTheDocument();
  });

  it("says nothing comes back when nothing has been translated", () => {
    render(
      <ReachPanel
        impact={impactWith({
          annotate: cls({ blocks: 4, collections: 1, projects: 1 }),
          transform: cls(),
          transform_projects: [],
        })}
      />,
    );
    expect(screen.getByText(/Nothing has been translated yet/)).toBeInTheDocument();
  });

  it("names whose desk a source rewrite lands on", () => {
    render(
      <ReachPanel
        impact={impactWith({
          annotate: cls({ blocks: 2, collections: 1, projects: 1 }),
          transform: cls({
            blocks: 6,
            collections: 2,
            projects: 2,
            targets: 14,
            approved: 9,
            locales: ["fr", "nb"],
          }),
          transform_projects: [
            { project_id: "p-web", project_name: "Marketing Website" },
            { project_id: "p-app", project_name: "" },
          ],
        })}
      />,
    );

    expect(screen.getByText("Source gets rewritten")).toBeInTheDocument();
    expect(screen.getByText(/That invalidates 14 translations in fr, nb/)).toBeInTheDocument();
    // A project with no name falls back to its id rather than rendering a gap.
    expect(screen.getByText(/Marketing Website, p-app/)).toBeInTheDocument();
  });

  it("says a stored report is a stored report rather than implying a fresh walk", () => {
    render(
      <ReachPanel
        impact={impactWith(
          {
            annotate: cls({ blocks: 5, targets: 3 }),
            transform: cls(),
            transform_projects: [],
          },
          { stored: true, computed_at: "2026-08-14T09:12:00Z" },
        )}
      />,
    );
    expect(screen.getByText(/Measured when this change-set was submitted/)).toBeInTheDocument();
  });

  it("explains a split that covers fewer rows than the hero counted", () => {
    render(
      <ReachPanel
        impact={impactWith(
          {
            annotate: cls({ blocks: 3, collections: 1, projects: 1 }),
            transform: cls(),
            transform_projects: [],
          },
          { affected_blocks: 9 },
        )}
      />,
    );
    expect(
      screen.getByText(/a block reached in several locales is one block to act on/),
    ).toBeInTheDocument();
  });
});
