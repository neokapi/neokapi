// The monolingual test (DECISIONS R13.2), as a test rather than an eyeball.
//
// A project with zero target languages must read as COMPLETE: the two-axis
// standing and the point map present, the language work (coverage, Bring up
// to date, Plan…, Review) absent behind the rail's Languages + door, and
// nothing on the page apologising for what it lacks. A source-only project is
// the ordinary case, not a half-configured one.

import { render, screen } from "./testUtils";
import { describe, it, expect, vi } from "vite-plus/test";

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

/** A status whose files carry French coverage, as a recipe that lists French
 *  on a collection without a target template would produce. */
const statusWithFrench: ProjectStatus = {
  ...status,
  collections: [
    { name: "Handbook", blockCount: 84, coverage: { "fr-FR": 42 }, targetLanguages: ["fr-FR"] },
  ],
};

function renderHome(project: KapiProject = monolingual, projectStatus: ProjectStatus = status) {
  return render(
    <ErrorProvider>
      <JobFeedProvider>
        <ActiveFilterProvider tabID="t1" enabled>
          <HomePage
            project={project}
            displayName="Handbook"
            tabID="t1"
            onNavigate={vi.fn()}
            status={projectStatus}
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
    expect(standing).toHaveTextContent("84 blocks");
    expect(screen.getByTestId("standing-voice")).toHaveTextContent("voice Handbook");
  });

  it("shows the point map, as one quiet row", () => {
    renderHome();
    const rows = screen.getAllByTestId("point-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("project default");
    expect(rows[0]).toHaveTextContent("Handbook");
  });

  it("keeps Bring up to date and Plan… out of sight until a target language exists", () => {
    renderHome();
    expect(screen.queryByText("Bring up to date")).not.toBeInTheDocument();
    expect(screen.queryByText("Plan…")).not.toBeInTheDocument();
    expect(document.querySelector('[data-slot="convergence-hero"]')).toBeNull();
    // The writing gate stays: Check leads to the checks and their fixes.
    expect(screen.getByText("Check")).toBeInTheDocument();
  });

  it("shows Bring up to date, Plan… and the targets once the recipe declares one", () => {
    renderHome({
      ...monolingual,
      defaults: { source_language: "en-US", target_languages: ["fr-FR"] },
    });
    expect(screen.getByText("Bring up to date")).toBeInTheDocument();
    expect(screen.getByText("Plan…")).toBeInTheDocument();
    expect(screen.getByTestId("project-standing")).toHaveTextContent("languages");
  });

  it("draws coverage only once the recipe declares a target language", () => {
    // French on a collection with no target template declares nothing.
    const { unmount } = renderHome(
      {
        ...monolingual,
        collections: [{ ...monolingual.collections![0], target_languages: ["fr-FR"] }],
      },
      statusWithFrench,
    );
    expect(screen.queryByText("Coverage across collections")).not.toBeInTheDocument();
    unmount();

    renderHome(
      { ...monolingual, defaults: { source_language: "en-US", target_languages: ["fr-FR"] } },
      statusWithFrench,
    );
    expect(screen.getByText("Coverage across collections")).toBeInTheDocument();
  });

  it("names the languages a collection declares, in the header and the standing", () => {
    renderHome({
      ...monolingual,
      collections: [
        {
          name: "Handbook",
          target_languages: ["de-DE"],
          content: [{ path: "docs/**/*.md", target: "out/{lang}/**/*.md" }],
        },
      ],
    });
    const header = screen.getByRole("heading", { name: "Handbook" }).parentElement!;
    expect(header).toHaveTextContent("de-DE");
    expect(header).not.toHaveTextContent("No targets");
    expect(screen.getByTestId("project-standing")).toHaveTextContent("languagesde-DE");
  });

  it("keeps the language surfaces quiet rather than empty", () => {
    renderHome();
    const standing = screen.getByTestId("project-standing");
    expect(standing).not.toHaveTextContent("languages");
    // The header names the source language and no target.
    expect(screen.queryByText("No targets")).not.toBeInTheDocument();
    expect(screen.queryByText("Coverage across collections")).not.toBeInTheDocument();

    render(<IconSidebar active="home" onChange={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Review" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Content Memory" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Languages +" })).toBeInTheDocument();
    // The Map, Context and the rest are fully present.
    expect(screen.getByRole("button", { name: "Map" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Context" })).toBeInTheDocument();
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
