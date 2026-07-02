import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { HomePage } from "../components/HomePage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { KapiProject, ProjectStatus, ConvergenceReport } from "../types/api";

function renderWithProviders(ui: React.ReactElement) {
  return render(<ErrorProvider>{ui}</ErrorProvider>);
}

const project: KapiProject = {
  version: "v1",
  name: "Demo",
  defaults: { source_language: "en-US", target_languages: ["fr-FR", "de-DE"] },
  content: [
    {
      name: "ui-strings",
      items: [{ path: "src/locales/en.json", format: { name: "json" } }],
    },
  ],
};

describe("HomePage merged collection surface", () => {
  it("renders per-collection stats and a project-wide coverage strip", () => {
    const status: ProjectStatus = {
      projectPath: "/p/demo.kapi",
      projectName: "Demo",
      hasData: true,
      collections: [
        {
          name: "ui-strings",
          blockCount: 100,
          coverage: { "fr-FR": 100, "de-DE": 50 },
          targetLanguages: ["fr-FR", "de-DE"],
        },
      ],
    };
    renderWithProviders(
      <HomePage
        project={project}
        displayName="Demo"
        tabID="t1"
        onNavigate={vi.fn()}
        status={status}
      />,
    );
    // The collection appears in the cake legend and its own row.
    expect(screen.getAllByText("ui-strings").length).toBeGreaterThan(0);
    // The cake legend shows the total block count.
    expect(screen.getByText("100 blocks")).toBeInTheDocument();
    // Per-language coverage (fr 100%, de 50%) shows in the strip + the aligned
    // per-language columns (2 languages ⇒ Option A bar columns).
    expect(screen.getAllByText("100%").length).toBeGreaterThan(0);
    expect(screen.getAllByText("50%").length).toBeGreaterThan(0);
  });

  it("prompts to run extract when nothing has been extracted yet", () => {
    const status: ProjectStatus = {
      projectPath: "/p/demo.kapi",
      projectName: "Demo",
      hasData: false,
      collections: [],
    };
    renderWithProviders(
      <HomePage
        project={project}
        displayName="Demo"
        tabID="t1"
        onNavigate={vi.fn()}
        status={status}
      />,
    );
    expect(screen.getByText(/Nothing extracted yet/)).toBeInTheDocument();
    expect(screen.getByText("Run extract")).toBeInTheDocument();
  });

  it("offers a Re-extract affordance once data exists", () => {
    const status: ProjectStatus = {
      projectPath: "/p/demo.kapi",
      projectName: "Demo",
      hasData: true,
      collections: [
        {
          name: "ui-strings",
          blockCount: 10,
          coverage: { "fr-FR": 10 },
          targetLanguages: ["fr-FR"],
        },
      ],
    };
    renderWithProviders(
      <HomePage
        project={project}
        displayName="Demo"
        tabID="t1"
        onNavigate={vi.fn()}
        status={status}
      />,
    );
    expect(screen.getByRole("button", { name: "Re-extract content" })).toBeInTheDocument();
  });

  it("flags a stale block store written by an older kapi", () => {
    const status: ProjectStatus = {
      projectPath: "/p/demo.kapi",
      projectName: "Demo",
      hasData: true,
      stale: true,
      collections: [
        {
          name: "ui-strings",
          blockCount: 10,
          coverage: { "fr-FR": 10 },
          targetLanguages: ["fr-FR"],
        },
      ],
    };
    renderWithProviders(
      <HomePage
        project={project}
        displayName="Demo"
        tabID="t1"
        onNavigate={vi.fn()}
        status={status}
      />,
    );
    expect(screen.getByText(/produced by an earlier version of kapi/)).toBeInTheDocument();
  });

  it("shows ship-gate ladder states from convergence instead of raw %", () => {
    const status: ProjectStatus = {
      projectPath: "/p/demo.kapi",
      projectName: "Demo",
      hasData: true,
      collections: [
        {
          name: "ui-strings",
          blockCount: 100,
          coverage: { "fr-FR": 100, "de-DE": 50 },
          targetLanguages: ["fr-FR", "de-DE"],
        },
      ],
    };
    const convergence: ConvergenceReport = {
      locales: [
        {
          collection: "ui-strings",
          locale: "fr-FR",
          total: 100,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "ui-strings",
          locale: "de-DE",
          total: 100,
          pct: { translated: 60, reviewed: 20 },
          gated: true,
          shippable: false,
        },
      ],
      review: [],
    };
    renderWithProviders(
      <HomePage
        project={project}
        displayName="Demo"
        tabID="t1"
        onNavigate={vi.fn()}
        status={status}
        convergence={convergence}
      />,
    );
    // fr-FR clears its gate → Shippable; de-DE has reviews but isn't shippable → In review.
    // (Both labels also appear in the timeline legend, hence getAllByText.)
    expect(screen.getAllByText("Shippable").length).toBeGreaterThan(0);
    expect(screen.getAllByText("In review").length).toBeGreaterThan(0);
    // The project-wide overview is now the per-language completeness timeline.
    expect(screen.getByText("Completeness by language")).toBeInTheDocument();
  });

  it("drops the standalone Content quick-action card (the page is content now)", () => {
    renderWithProviders(
      <HomePage project={project} displayName="Demo" tabID="t1" onNavigate={vi.fn()} />,
    );
    // Quick actions: Check / Flows / Tools / Settings — no "Content" card.
    expect(screen.getByText("Check")).toBeInTheDocument();
    expect(screen.queryByText("Content")).not.toBeInTheDocument();
  });
});
