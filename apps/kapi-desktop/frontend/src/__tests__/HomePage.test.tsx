import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { HomePage } from "../components/HomePage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { KapiProject, ProjectStatus } from "../types/api";

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
    // The collection card carries its own name + block-count badge.
    expect(screen.getByText("ui-strings")).toBeInTheDocument();
    expect(screen.getByText("100 blocks")).toBeInTheDocument();
    // The slim coverage strip shows per-language percentages (fr 100%, de 50%).
    expect(screen.getByText("100%")).toBeInTheDocument();
    expect(screen.getByText("50%")).toBeInTheDocument();
    // The per-collection header bar shows the mean coverage (75%).
    expect(screen.getByText("75%")).toBeInTheDocument();
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

  it("drops the standalone Content quick-action card (the page is content now)", () => {
    renderWithProviders(
      <HomePage project={project} displayName="Demo" tabID="t1" onNavigate={vi.fn()} />,
    );
    // Quick actions: Check / Flows / Tools / Settings — no "Content" card.
    expect(screen.getByText("Check")).toBeInTheDocument();
    expect(screen.queryByText("Content")).not.toBeInTheDocument();
  });
});
