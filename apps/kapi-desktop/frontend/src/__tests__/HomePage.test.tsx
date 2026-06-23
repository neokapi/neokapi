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
  content: [{ path: "src/locales/en.json", format: { name: "json" } }],
};

describe("HomePage content status", () => {
  it("renders per-collection, per-locale coverage", () => {
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
    expect(screen.getByText("Content Overview")).toBeInTheDocument();
    // The collection appears in the distribution legend and its compact row.
    expect(screen.getAllByText("ui-strings").length).toBeGreaterThan(0);
    // Cross-collection coverage summary: fr-FR fully translated, de-DE half.
    expect(screen.getByText("100 / 100 (100%)")).toBeInTheDocument();
    expect(screen.getByText("50 / 100 (50%)")).toBeInTheDocument();
  });

  it("shows a 'run extract' prompt when nothing has been extracted", () => {
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
    expect(screen.getByText("Nothing extracted yet.")).toBeInTheDocument();
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
});
