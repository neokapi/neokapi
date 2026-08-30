// The monolingual test (DECISIONS R13.2), as a test rather than an eyeball.
//
// A project with zero target languages must read as COMPLETE: the two-axis
// standing, the point map and the convergence hero all present, the surfaces
// its languages do not call for simply absent, and nothing on the page
// apologising for what it lacks. A source-only project is the ordinary case,
// not a half-configured one.

import { render, screen } from "./testUtils";
import { describe, it, expect, vi } from "vitest";

vi.mock("../hooks/useApi", async () => {
  const actual = await vi.importActual<typeof import("../hooks/useApi")>("../hooks/useApi");
  return {
    ...actual,
    api: {
      ...actual.api,
      getSampleInfo: vi.fn().mockResolvedValue(null),
      getProjectStatus: vi.fn().mockResolvedValue(null),
      getConvergence: vi.fn().mockResolvedValue(null),
      getConvergePlan: vi.fn().mockResolvedValue(null),
      listFormats: vi.fn().mockResolvedValue([]),
      getBasePath: vi.fn().mockResolvedValue("/p"),
      getHomeDir: vi.fn().mockResolvedValue("/home/dev"),
      getKnownLocales: vi.fn().mockResolvedValue([]),
      matchContent: vi.fn().mockResolvedValue([]),
      listProjectFiles: vi.fn().mockResolvedValue([]),
      listOutputs: vi.fn().mockResolvedValue({}),
      listFlows: vi.fn().mockResolvedValue([]),
    },
    call: vi.fn().mockResolvedValue(null),
  };
});

import { HomePage } from "../components/HomePage";
import { ErrorProvider } from "../components/ErrorBanner";
import { IconSidebar } from "../components/IconSidebar";
import { ActiveFilterProvider } from "../context/ActiveFilterContext";
import { JobFeedProvider } from "../context/JobFeedContext";
import type { KapiProject, ProjectStatus } from "../types/api";
import type { ProjectPointsResult } from "../components/ProjectStanding";

/** A real source-only project: one collection, one language, no targets. */
const monolingual: KapiProject = {
  version: "v1",
  name: "Handbook",
  defaults: { source_language: "en-US" },
  collections: [{ name: "Handbook", content: [{ path: "docs/**/*.md" }] }],
};

const status: ProjectStatus = {
  projectPath: "/p/handbook/kapi.yaml",
  projectName: "Handbook",
  hasData: true,
  collections: [{ name: "Handbook", blockCount: 84, coverage: {}, targetLanguages: [] }],
};

const points: ProjectPointsResult = {
  at: "2026-08-30T09:00:00Z",
  points: [
    {
      ref: "",
      label: "project default",
      default: true,
      collections: ["Handbook"],
      voice: "Handbook",
      voice_field: "defaults.voice",
    },
  ],
};

function renderHome() {
  return render(
    <ErrorProvider>
      <JobFeedProvider>
        <ActiveFilterProvider tabID="t1" enabled>
          <HomePage
            project={monolingual}
            displayName="Handbook"
            tabID="t1"
            onNavigate={vi.fn()}
            status={status}
            points={points}
            server={{ connected: false, stream: "main" }}
            basePath="/p/handbook"
            formatList={[]}
          />
        </ActiveFilterProvider>
      </JobFeedProvider>
    </ErrorProvider>,
  );
}

describe("the monolingual home", () => {
  it("shows the two-axis standing", () => {
    renderHome();
    const standing = screen.getByTestId("project-standing");
    expect(standing).toHaveTextContent("84 unit(s) extracted");
    expect(screen.getByTestId("standing-voice")).toHaveTextContent("voice Handbook");
  });

  it("shows the point map, as one quiet row", () => {
    renderHome();
    const rows = screen.getAllByTestId("point-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("project default");
    expect(rows[0]).toHaveTextContent("Handbook");
  });

  it("shows the convergence hero", () => {
    renderHome();
    expect(screen.getByText("Bring up to date")).toBeInTheDocument();
  });

  it("keeps the language surfaces quiet rather than empty", () => {
    renderHome();
    const standing = screen.getByTestId("project-standing");
    expect(standing).not.toHaveTextContent("languages");

    render(<IconSidebar mode="projects" active="home" onChange={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Review" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Content Memory" })).not.toBeInTheDocument();
    // Context, Checks and the rest are fully present.
    expect(screen.getByRole("button", { name: "Context" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Checks" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Toolbox" })).toBeInTheDocument();
  });

  it("apologises for nothing", () => {
    renderHome();
    const page = screen.getByTestId("project-standing").closest("div.p-6") ?? document.body;
    const text = page.textContent ?? "";
    for (const apology of [
      "No target languages",
      "no languages configured",
      "Add a target language",
      "not configured",
      "nothing to show",
    ]) {
      expect(text).not.toContain(apology);
    }
  });
});
