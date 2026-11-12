// The monolingual test (DECISIONS R13.2), as a test rather than an eyeball.
//
// A project with zero target languages must read as COMPLETE: the two-axis
// standing and the point map present on Project Settings → Content, the
// language work (coverage, Bring up to date, Plan…) absent from the Overview
// and Content, and nothing on either page apologising for what it lacks. A source-only project is
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

import { OverviewStanding } from "../components/OverviewStanding";
import { ProjectContentPage } from "../components/ProjectContentPage";
import { declaresTargetLanguages } from "../lib/languages";
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
          <OverviewStanding
            tabID="t1"
            hasTargetLanguages={declaresTargetLanguages(project)}
            onNavigate={vi.fn()}
          />
          <ProjectContentPage
            project={project}
            displayName="Handbook"
            tabID="t1"
            onUpdate={vi.fn()}
            hasTargetLanguages={declaresTargetLanguages(project)}
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
  });

  it("shows the hero, Plan… and the targets once the recipe declares one", () => {
    renderHome({
      ...monolingual,
      defaults: { source_language: "en-US", target_languages: ["fr-FR"] },
    });
    expect(document.querySelector('[data-slot="convergence-hero"]')).toBeInTheDocument();
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

  it("names the languages a collection declares in the standing", () => {
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
    expect(screen.getByTestId("project-standing")).toHaveTextContent("languagesde-DE");
  });

  it("keeps the language surfaces quiet rather than empty", () => {
    renderHome();
    const standing = screen.getByTestId("project-standing");
    expect(standing).not.toHaveTextContent("languages");
    expect(screen.queryByText("Coverage across collections")).not.toBeInTheDocument();

    render(<IconSidebar active="home" onChange={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Content Memory" })).not.toBeInTheDocument();
    // The Overview, Context and the rest are fully present.
    expect(screen.getByRole("button", { name: "Overview" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Context" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Project Settings" })).toBeInTheDocument();
  });

  it("apologises for nothing", () => {
    renderHome();
    const text = document.body.textContent ?? "";
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
