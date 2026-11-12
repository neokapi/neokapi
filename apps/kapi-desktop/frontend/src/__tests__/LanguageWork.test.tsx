// Languages are the second act. A project whose recipe declares no target
// language shows no language work: Review opens the add-a-language settings
// under one line, and the Overview carries no language standing. Once the
// saved recipe on the tab declares a target, the Overview shows its language
// standing, with its run action, and Review holds the queue. The runner's
// Catch up view reads the same answer. A saved session's retired
// "languages" and "project-home" views open the Overview.

import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";

vi.mock("../hooks/useApi", async () => {
  const actual = await vi.importActual<typeof import("../hooks/useApi")>("../hooks/useApi");
  return {
    ...actual,
    api: {
      ...actual.api,
      getSampleInfo: vi.fn().mockResolvedValue(null),
      getProjectStatus: vi.fn().mockResolvedValue(null),
      getConvergence: vi.fn().mockResolvedValue({ locales: [], review: [] }),
      // Three blocks to draft, so the Overview offers its run action.
      getConvergePlan: vi.fn().mockResolvedValue({
        plan: {
          scopes: [],
          totals: { missing_target: 3, memory_exact: 0, ai_remaining: 3, token_estimate: 30 },
        },
      }),
      getProjectServer: vi.fn().mockResolvedValue(null),
      getLastRunError: vi.fn().mockResolvedValue(null),
      listFormats: vi.fn().mockResolvedValue([]),
      getBasePath: vi.fn().mockResolvedValue("/p"),
      getHomeDir: vi.fn().mockResolvedValue("/home/dev"),
      getKnownLocales: vi.fn().mockResolvedValue([]),
      matchContent: vi.fn().mockResolvedValue([]),
      listProjectFiles: vi.fn().mockResolvedValue([]),
      listOutputs: vi.fn().mockResolvedValue({}),
      listFlows: vi.fn().mockResolvedValue([]),
      listProjectTools: vi.fn().mockResolvedValue([]),
      getEffectiveModel: vi.fn().mockResolvedValue(null),
      syncProjectStore: vi.fn().mockResolvedValue(null),
      aiNeedsModelChoice: vi.fn().mockResolvedValue(false),
      bringUpToDate: vi.fn().mockResolvedValue(undefined),
    },
    call: vi.fn().mockResolvedValue(null),
  };
});

// The Overview's own reads are OverviewPage's tests; here it draws what the route
// hands it to draw under its counts.
vi.mock("../components/OverviewPage", () => ({
  OverviewPage: ({ standing }: { standing?: React.ReactNode }) => (
    <div data-testid="overview">{standing}</div>
  ),
}));

import { ViewSwitch } from "../components/ViewSwitch";
import { ErrorProvider } from "../components/ErrorBanner";
import { ActiveFilterProvider } from "../context/ActiveFilterContext";
import { JobFeedProvider } from "../context/JobFeedContext";
import { declaresTargetLanguages, resolvedTargetLanguages } from "../lib/languages";
import type { ProjectHistory } from "../hooks/useProjectHistory";
import type { TabState } from "../hooks/useTabManager";
import type { KapiProject } from "../types/api";

const sourceOnly: KapiProject = {
  version: "v1",
  name: "Handbook",
  defaults: { source_language: "en-US" },
  collections: [{ name: "Handbook", content: [{ path: "docs/**/*.md" }] }],
};

const withFrench: KapiProject = {
  ...sourceOnly,
  defaults: { source_language: "en-US", target_languages: ["fr-FR"] },
};

function tabOf(project: KapiProject): TabState {
  return {
    info: { id: "t1", name: "Handbook", path: "/p/handbook/kapi.yaml", recipe_written: true },
    project,
    pluginsResolved: true,
    view: "map",
  };
}

function historyOf(project: KapiProject): ProjectHistory {
  return {
    project,
    set: vi.fn(),
    replace: vi.fn(),
    undo: vi.fn(),
    redo: vi.fn(),
    markSaved: vi.fn(),
    isDirty: false,
    canUndo: false,
    canRedo: false,
  };
}

function view(effectiveView: string, project: KapiProject, navigate = vi.fn()) {
  return (
    <ErrorProvider>
      <JobFeedProvider>
        <ActiveFilterProvider tabID="t1" enabled>
          <ViewSwitch
            effectiveView={effectiveView}
            activeTab={tabOf(project)}
            history={historyOf(project)}
            updateProject={vi.fn()}
            navigate={navigate}
            updateTab={vi.fn()}
            workspace={null}
            startHereDismissed={false}
            onDismissStartHere={vi.fn()}
            onRestoreStartHere={vi.fn()}
            hasOpenProject
            onOpenWhatsNew={vi.fn()}
            onOpenCheckout={vi.fn()}
            onOpenContext={vi.fn()}
            onForgetProject={vi.fn()}
            onNewProject={vi.fn()}
            onOpenProject={vi.fn()}
            onCreateSampleProject={vi.fn()}
            onResetSample={vi.fn()}
            onOpenPath={vi.fn()}
            openScreen={false}
          />
        </ActiveFilterProvider>
      </JobFeedProvider>
    </ErrorProvider>
  );
}

const LINE =
  "Add a language to translate into. Translations to review appear here, with content memory and coverage.";

describe("language work", () => {
  it("opens the add-a-language settings on Review while there is nothing to review", () => {
    render(view("review", sourceOnly));
    expect(screen.getByRole("heading", { name: "Review" })).toBeInTheDocument();
    expect(screen.getByText(LINE)).toBeInTheDocument();
    expect(screen.getByText("Translate into")).toBeInTheDocument();
  });

  it("keeps the Overview free of language work until the recipe declares a target", () => {
    const { rerender } = render(view("map", sourceOnly));
    expect(screen.getByTestId("overview")).toBeInTheDocument();
    expect(screen.queryByText("Plan…")).not.toBeInTheDocument();

    // A saved recipe that gains a target updates the tab's project; the
    // route follows without the tab being reopened.
    rerender(view("map", withFrench));
    expect(screen.getByText("Plan…")).toBeInTheDocument();
  });

  it.each(["languages", "project-home", "content"])(
    "opens the Overview for a saved %s view",
    (saved) => {
      render(view(saved, withFrench));
      expect(screen.getByTestId("overview")).toBeInTheDocument();
    },
  );
});

describe("Catch up in the runner", () => {
  it("opens the Catch up view only while the saved recipe declares a target", async () => {
    const navigate = vi.fn();
    const { rerender } = render(view("map", withFrench, navigate));
    const run = await screen.findByRole("button", { name: "Translate 3 blocks" });
    await waitFor(() => expect(run).toBeEnabled());
    await userEvent.click(run);
    await waitFor(() => expect(navigate).toHaveBeenLastCalledWith("runner"));

    rerender(view("runner", withFrench, navigate));
    expect(screen.getByRole("heading", { name: "Catch up" })).toBeInTheDocument();

    // The saved recipe loses its last target: the route shows the run it
    // launched as the selected job, with no Catch up view.
    rerender(view("runner", sourceOnly, navigate));
    expect(screen.queryByRole("heading", { name: "Catch up" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Run: up" })).toBeInTheDocument();
  });
});

describe("declaresTargetLanguages", () => {
  it.each<[string, KapiProject | null, boolean]>([
    ["no project", null, false],
    ["source only", sourceOnly, false],
    ["a default target", withFrench, true],
    [
      "an item with a target template and its own languages",
      {
        ...sourceOnly,
        collections: [
          { content: [{ path: "a/*.json", target: "b/{lang}", target_languages: ["de"] }] },
        ],
      },
      true,
    ],
    [
      "an item with a target template and the collection's languages",
      {
        ...sourceOnly,
        collections: [
          { target_languages: ["de"], content: [{ path: "a/*.json", target: "b/{lang}" }] },
        ],
      },
      true,
    ],
    [
      "a bare entry with a target template and languages",
      {
        ...sourceOnly,
        collections: [{ path: "a/*.json", target: "b/{lang}", target_languages: ["de"] }],
      },
      true,
    ],
    [
      "collection languages with no target template",
      {
        ...sourceOnly,
        collections: [{ target_languages: ["de"], content: [{ path: "a/*.json" }] }],
      },
      false,
    ],
    [
      "a target template with no languages",
      { ...sourceOnly, collections: [{ content: [{ path: "a/*.json", target: "b/{lang}" }] }] },
      false,
    ],
  ])("%s", (_name, project, want) => {
    expect(declaresTargetLanguages(project)).toBe(want);
    // The resolved list is empty exactly when nothing is declared.
    expect(resolvedTargetLanguages(project).length > 0).toBe(want);
  });
});

describe("resolvedTargetLanguages", () => {
  it("lists the defaults, then each item's own languages before its collection's, once each", () => {
    const project: KapiProject = {
      ...withFrench,
      collections: [
        {
          target_languages: ["de-DE", "fr-FR"],
          content: [
            { path: "a/*.json", target: "b/{lang}" },
            { path: "c/*.json", target: "d/{lang}", target_languages: ["ja-JP"] },
            { path: "e/*.json", target_languages: ["ko-KR"] },
          ],
        },
      ],
    };
    expect(resolvedTargetLanguages(project)).toEqual(["fr-FR", "de-DE", "ja-JP"]);
  });
});
