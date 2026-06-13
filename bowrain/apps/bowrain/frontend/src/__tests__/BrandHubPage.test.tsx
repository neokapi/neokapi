import { describe, it, expect, vi } from "vitest";
import { useState } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import {
  ApiProvider,
  WorkspaceProvider,
  TooltipProvider,
  type ApiAdapter,
  type Workspace,
  type ProjectInfo,
} from "@neokapi/ui";

// Stub the heavy shared brand-hub views so the test isolates BrandHubPage's
// section/selection routing (not the views' own data fetching). Each stub
// exposes the callbacks BrandHubPage wires so the open/back flows are testable.
vi.mock("@neokapi/ui", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@neokapi/ui")>();
  return {
    ...actual,
    ConceptsView: ({ onOpenConcept }: { onOpenConcept: (id: string) => void }) => (
      <button data-testid="concepts-view" onClick={() => onOpenConcept("c1")}>
        concepts
      </button>
    ),
    ConceptStoryView: ({ conceptId, onBack }: { conceptId: string; onBack: () => void }) => (
      <div data-testid="story-view">
        <span>story:{conceptId}</span>
        <button onClick={onBack}>back-concept</button>
      </div>
    ),
    ExperimentsView: ({ onOpenExperiment }: { onOpenExperiment: (id: string) => void }) => (
      <button data-testid="experiments-view" onClick={() => onOpenExperiment("cs1")}>
        experiments
      </button>
    ),
    ExperimentDetailView: ({
      changesetId,
      onBack,
    }: {
      changesetId: string;
      onBack: () => void;
    }) => (
      <div data-testid="experiment-detail">
        <span>detail:{changesetId}</span>
        <button onClick={onBack}>back-experiment</button>
      </div>
    ),
    ActivityView: () => <div data-testid="activity-view" />,
    BrandDashboardView: ({ onViewConcepts }: { onViewConcepts: () => void }) => (
      <button data-testid="dashboard-view" onClick={onViewConcepts}>
        dashboard
      </button>
    ),
  };
});

// The Voice section re-homes the existing BrandPage review surface.
vi.mock("../components/BrandPage", () => ({
  BrandPage: () => <div data-testid="voice-view" />,
}));

import { BrandHubPage, type BrandSection } from "../components/BrandHubPage";

const teamWorkspace: Workspace = {
  id: "ws1",
  name: "Acme",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "team",
  role: "owner",
};

const projects = [{ id: "proj-1", name: "Marketing Site" } as unknown as ProjectInfo];

// A minimal adapter; the stubbed views never call it.
const adapter = {} as unknown as ApiAdapter;

/**
 * Mirrors App.tsx's brand state ownership so the open/back/goto callbacks drive
 * the section + drill-down selection exactly as the desktop app does.
 */
function Harness({
  initialSection = "concepts",
  ws = teamWorkspace,
}: {
  initialSection?: BrandSection;
  ws?: Workspace;
}) {
  const [section, setSection] = useState<BrandSection>(initialSection);
  const [conceptId, setConceptId] = useState("");
  const [changesetId, setChangesetId] = useState("");
  return (
    <TooltipProvider>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={ws}>
          <BrandHubPage
            projects={projects}
            section={section}
            conceptId={conceptId}
            changesetId={changesetId}
            onOpenConcept={(cid) => {
              setSection("concepts");
              setConceptId(cid);
              setChangesetId("");
            }}
            onCloseConcept={() => setConceptId("")}
            onOpenExperiment={(id) => {
              setSection("experiments");
              setChangesetId(id);
              setConceptId("");
            }}
            onCloseExperiment={() => setChangesetId("")}
            onGotoSection={(s) => {
              setSection(s);
              setConceptId("");
              setChangesetId("");
            }}
          />
        </WorkspaceProvider>
      </ApiProvider>
    </TooltipProvider>
  );
}

describe("BrandHubPage", () => {
  it("renders a connect prompt for a personal workspace", () => {
    render(<Harness ws={{ ...teamWorkspace, type: "personal" }} />);
    expect(screen.getByTestId("brand-hub-empty")).toBeInTheDocument();
    expect(screen.queryByTestId("concepts-view")).not.toBeInTheDocument();
  });

  it("opens a concept story from the Concepts list and navigates back", () => {
    render(<Harness initialSection="concepts" />);
    expect(screen.getByTestId("concepts-view")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("concepts-view"));
    expect(screen.getByTestId("story-view")).toHaveTextContent("story:c1");

    fireEvent.click(screen.getByText("back-concept"));
    expect(screen.getByTestId("concepts-view")).toBeInTheDocument();
  });

  it("opens an experiment detail from the Experiments list and navigates back", () => {
    render(<Harness initialSection="experiments" />);
    expect(screen.getByTestId("experiments-view")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("experiments-view"));
    expect(screen.getByTestId("experiment-detail")).toHaveTextContent("detail:cs1");

    fireEvent.click(screen.getByText("back-experiment"));
    expect(screen.getByTestId("experiments-view")).toBeInTheDocument();
  });

  it("renders the Voice review surface", () => {
    render(<Harness initialSection="voice" />);
    expect(screen.getByTestId("voice-view")).toBeInTheDocument();
  });

  it("renders Activity", () => {
    render(<Harness initialSection="activity" />);
    expect(screen.getByTestId("activity-view")).toBeInTheDocument();
  });

  it("jumps from the Dashboard to the Concepts section", () => {
    render(<Harness initialSection="dashboard" />);
    expect(screen.getByTestId("dashboard-view")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("dashboard-view"));
    expect(screen.getByTestId("concepts-view")).toBeInTheDocument();
  });
});
