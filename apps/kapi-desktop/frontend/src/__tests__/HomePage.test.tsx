import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";
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
  collections: [
    {
      name: "ui-strings",
      content: [{ path: "src/locales/en.json", format: { name: "json" } }],
    },
  ],
};

describe("HomePage merged collection surface", () => {
  it("renders per-collection stats and a project-wide coverage strip", () => {
    const status: ProjectStatus = {
      projectPath: "/p/demo/kapi.yaml",
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
    // The project line and the cake legend both show the total block count.
    expect(screen.getAllByText("100 blocks")).toHaveLength(2);
    // Per-language coverage (fr 100%, de 50%) shows in the strip + the aligned
    // per-language columns (2 languages ⇒ Option A bar columns).
    expect(screen.getAllByText("100%").length).toBeGreaterThan(0);
    expect(screen.getAllByText("50%").length).toBeGreaterThan(0);
  });

  it("reads the files itself when nothing has been read yet", () => {
    const status: ProjectStatus = {
      projectPath: "/p/demo/kapi.yaml",
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
    expect(screen.getByText("Reading your files…")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /extract/i })).not.toBeInTheDocument();
  });

  it("keeps reading the files again behind the Advanced disclosure", async () => {
    const status: ProjectStatus = {
      projectPath: "/p/demo/kapi.yaml",
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
    // The app reads changed files on its own, so the manual override lives
    // only inside the Advanced disclosure.
    expect(screen.queryByRole("button", { name: "Read every file again" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Advanced actions" }));
    expect(screen.getByRole("button", { name: "Read every file again" })).toBeInTheDocument();
  });

  it("no stale-store banner — Bring up to date auto-heals version drift", () => {
    const status: ProjectStatus = {
      projectPath: "/p/demo/kapi.yaml",
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
    expect(screen.queryByText(/produced by an earlier version of kapi/)).not.toBeInTheDocument();
  });

  it("shows ship-gate ladder states from convergence instead of raw %", () => {
    const status: ProjectStatus = {
      projectPath: "/p/demo/kapi.yaml",
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
          pct: { translated: 100, established: 100 },
          gated: true,
          shippable: true,
          shipState: "established",
        },
        {
          collection: "ui-strings",
          locale: "de-DE",
          total: 100,
          pct: { translated: 60, established: 20 },
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
    // fr-FR clears its established gate → Established; de-DE has established
    // units but clears no gate → In review. ("In review" also appears in the
    // timeline legend, hence getAllByText.)
    expect(screen.getAllByText("Approved").length).toBeGreaterThan(0);
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
