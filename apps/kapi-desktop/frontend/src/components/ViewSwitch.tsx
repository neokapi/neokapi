import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import type { ContextProjectStanding, KapiProject, FlowSpec, WorkspaceHome } from "../types/api";
import { ProjectErrorBoundary } from "./ProjectErrorBoundary";
import type { TabState } from "../hooks/useTabManager";
import type { ProjectHistory } from "../hooks/useProjectHistory";
import { AppHome } from "./AppHome";
import { RunnerPage } from "./RunnerPage";
import { FlowsPage } from "./FlowsPage";
import { ToolRunnerPage } from "./ToolRunnerPage";
import { ToolboxPage } from "./ToolboxPage";
import { TermsPage } from "./TermsPage";
import { MemoriesPage } from "./MemoriesPage";
import { ChecksPanel } from "./ChecksPanel";
import { MapPage } from "./MapPage";
import { ReaderPage, readerTargetOf, readerView } from "./ReaderPage";
import { ContextHub, contextSectionOf, decideRowOf, type ContextPin } from "./ContextHub";
import { HistoryView } from "./HistoryView";
import { ReviewPage, type ReviewScope } from "./ReviewPage";
import { FormatsPage } from "./FormatsPage";
import { SettingsPage } from "./SettingsPage";
import { HomePage } from "./HomePage";
import { ProjectSetupPage } from "./ProjectSetupPage";
import { ProjectSettingsPage } from "./ProjectSettingsPage";
import { ProjectPresetPage } from "./ProjectPresetPage";
import { ProjectSettingsTabs, settingsSectionOf } from "./ProjectSettingsTabs";
import { OpenScreen } from "./OpenScreen";
import { LanguagesPage } from "./LanguagesPage";
import { declaresTargetLanguages } from "../lib/languages";
import { useJobFeed } from "../context/JobFeedContext";

/**
 * Renders RunnerPage for a job selected from the feed (no runnerState).
 * Falls back to project-home if there's no selected job to display.
 */
function RunnerViewFallback({
  tabID,
  project,
  navigate,
}: {
  tabID: string;
  project: KapiProject;
  navigate: (view: string) => void;
}) {
  const { selectedJob } = useJobFeed();

  useEffect(() => {
    if (!selectedJob) {
      navigate("project-home");
    }
  }, [selectedJob, navigate]);

  if (!selectedJob) return null;

  return (
    <RunnerPage
      tabID={tabID}
      flowName={selectedJob.flowName}
      project={project}
      onClose={() => navigate("project-home")}
    />
  );
}

interface ViewSwitchProps {
  effectiveView: string;
  activeTab: TabState | null;
  history: ProjectHistory;
  updateProject: (project: KapiProject) => void;
  navigate: (view: string) => void;
  updateTab: (id: string, patch: Partial<TabState>) => void;
  // Home page props
  workspace: WorkspaceHome | null;
  workspaceError?: unknown;
  /** Every project's context in numbers. */
  standings?: ContextProjectStanding[] | null;
  /** The home's Start here steps were dismissed. */
  startHereDismissed: boolean;
  onDismissStartHere: () => void;
  onRestoreStartHere: () => void;
  /** A project tab is open, so the home can connect an assistant to it. */
  hasOpenProject: boolean;
  /** Open the most recent project tab on its Assistant section. */
  onConnectAssistant: () => void;
  /** Open a project on its Decide section, by workspace key. */
  onOpenWhatsNew: (key: string) => void;
  /** Open a project from one of its checkouts, by recipe path. */
  onOpenCheckout: (recipe: string) => void;
  /** Open a project with no checkout here, on its context alone. */
  onOpenContext: (key: string) => void;
  /** Remove a project and the context the workspace holds for it. */
  onForgetProject: (key: string) => Promise<void> | void;
  onNewProject: () => void;
  onOpenProject: () => void;
  onCreateSampleProject: (name: string) => void;
  /** Reset an out-of-date sample project to the version bundled with this kapi. */
  onResetSample: (tabID: string) => void;
  /** Open a folder, a recipe or a file inside a folder, by path. */
  onOpenPath: (path: string) => void;
  /**
   * The home is the Open screen rather than the workspace overview: fewer than
   * two projects are registered, so there is nothing to compare.
   */
  openScreen: boolean;
}

export function ViewSwitch({
  effectiveView,
  activeTab,
  history,
  updateProject,
  navigate,
  updateTab,
  workspace,
  workspaceError,
  standings,
  startHereDismissed,
  onDismissStartHere,
  onRestoreStartHere,
  hasOpenProject,
  onConnectAssistant,
  onOpenWhatsNew,
  onOpenCheckout,
  onOpenContext,
  onForgetProject,
  onNewProject,
  onOpenProject,
  onCreateSampleProject,
  onResetSample,
  onOpenPath,
  openScreen,
}: ViewSwitchProps) {
  // State for the runner view — set when the user clicks Run on a flow. runId is
  // a fresh token per Run click so the runner auto-launches exactly once per
  // request (not once per mount): re-navigating to the runner remounts the page
  // but must not relaunch the flow.
  const [runnerState, setRunnerState] = useState<{
    flowName: string;
    flow?: FlowSpec;
    runId: number;
    scopePaths?: string[];
    scopeLabel?: string;
    /** Convergence run (Bring up to date): the runner renders passes and
     *  launches through the shared `kapi up` engine. */
    converge?: boolean;
  } | null>(null);
  const runCounter = useRef(0);
  const launchedRunIdRef = useRef<number | null>(null);

  // Review-surface entry scope: a ship-gate cell / timeline tag on the
  // Collections surface opens Review narrowed to its (collection, locale). A
  // counter keys the page so re-entering with a new scope resets its filters.
  const [reviewScope, setReviewScope] = useState<ReviewScope | null>(null);
  const reviewEntry = useRef(0);
  const handleOpenReview = useCallback(
    (scope?: ReviewScope) => {
      reviewEntry.current += 1;
      setReviewScope(scope ?? null);
      navigate("review");
    },
    [navigate],
  );

  // Context-surface entry pin: a check finding names the rule that fired and
  // the point it is scoped to, and opens the explorer standing there. A counter
  // keys the hub so re-entering with a new pin resets it.
  const [contextPin, setContextPin] = useState<ContextPin | null>(null);
  const contextEntry = useRef(0);
  const handleOpenContext = useCallback(
    (pin?: ContextPin) => {
      contextEntry.current += 1;
      setContextPin(pin ?? null);
      navigate("context");
    },
    [navigate],
  );
  // The Map's Decide opens the hub's Decide section. A suggestion's own Decide
  // names its row in the view ("context:rules:<row key>"), so the row travels
  // with the route and a later visit from the rail opens on no row of its own.
  const handleDecide = useCallback(
    (rowKey?: string) => {
      contextEntry.current += 1;
      setContextPin(null);
      navigate(rowKey ? `context:rules:${rowKey}` : "context:rules");
    },
    [navigate],
  );

  // The Reader opens on one file from the Map's Files rows and from a finding
  // on the Checks page, and its back button returns to the page it came from.
  const [readerBack, setReaderBack] = useState<string>("map");
  const handleOpenReader = useCallback(
    (path: string, blockID?: string) => {
      setReaderBack(effectiveView.split(":")[0] === "checks" ? "checks" : "map");
      navigate(readerView(path, blockID));
    },
    [effectiveView, navigate],
  );

  // Run a project flow from the home page: navigate to the runner view. An
  // optional scope (the per-collection "Run") restricts the run to a single
  // collection's files.
  const handleRunFlow = useCallback(
    (_flowName: string, spec: FlowSpec, opts?: { scopePaths?: string[]; scopeLabel?: string }) => {
      runCounter.current += 1;
      setRunnerState({
        flowName: _flowName,
        flow: spec,
        runId: runCounter.current,
        scopePaths: opts?.scopePaths,
        scopeLabel: opts?.scopeLabel,
      });
      navigate("runner");
    },
    [navigate],
  );

  // Bring up to date (the home hero / plan dialog): the HERO launches the run
  // (so a synchronous launch error stays inline on the hero); this handler
  // fires only after a successful launch and merely opens the runner's
  // convergence view on the already-running job. Marking the runId consumed
  // keeps RunnerPage from relaunching (and duplicating) the run on mount.
  const handleBringUpToDate = useCallback(() => {
    const project = history.project;
    const flowName = project?.defaults?.flow ?? "";
    runCounter.current += 1;
    launchedRunIdRef.current = runCounter.current;
    setRunnerState({
      flowName,
      flow: flowName ? project?.flows?.[flowName] : undefined,
      runId: runCounter.current,
      converge: true,
    });
    navigate("runner");
  }, [history, navigate]);

  // Home, a global overlay over every tab: the Open screen until two projects
  // are registered, the workspace overview from then on.
  if (effectiveView === "home") {
    if (openScreen) {
      return (
        <OpenScreen
          workspace={workspace}
          onChooseFolder={onOpenProject}
          onOpenPath={onOpenPath}
          onTrySample={() => onCreateSampleProject("kapimart")}
          onOpenSettings={() => navigate("app-settings")}
        />
      );
    }
    return (
      <AppHome
        workspace={workspace}
        workspaceError={workspaceError}
        standings={standings}
        startHereDismissed={startHereDismissed}
        onDismissStartHere={onDismissStartHere}
        hasOpenProject={hasOpenProject}
        onConnectAssistant={onConnectAssistant}
        onOpenWhatsNew={onOpenWhatsNew}
        onOpenCheckout={onOpenCheckout}
        onOpenContext={onOpenContext}
        onForgetProject={onForgetProject}
        onNewProject={onNewProject}
        onOpenProject={onOpenProject}
        onNavigate={navigate}
        onCreateSampleProject={onCreateSampleProject}
      />
    );
  }

  // The workspace's history — a global overlay like the home it opens from.
  if (effectiveView === "history") {
    return <HistoryView onBack={() => navigate("home")} />;
  }

  const settings = (
    <SettingsPage startHereDismissed={startHereDismissed} onRestoreStartHere={onRestoreStartHere} />
  );

  // Global app settings, from the gear at the bottom of the sidebar
  if (effectiveView === "app-settings") {
    return settings;
  }

  // Project views — require an active tab
  if (!activeTab) return null;

  const tabID = activeTab.info.id;
  // Whether the saved recipe declares a target language. The rail reads the
  // same answer from the same recipe, so the Languages door, Review, Content
  // memory and the Project page's language standing appear together.
  const languages = declaresTargetLanguages(activeTab.project);
  // "context:<section>" opens the hub on one section; the view's base is the route.
  const route = effectiveView.split(":")[0];
  const hubSection = contextSectionOf(effectiveView);

  // A context-only tab has no recipe and no files, so every project surface
  // but the Context hub has nothing to read. It opens on the hub and stays
  // there; the banner says why the rest is absent.
  if (activeTab.info.context_only) {
    return (
      <div className="flex h-full min-h-0 flex-col">
        <div className="shrink-0 border-b border-border bg-muted/30 px-6 py-2 text-xs text-muted-foreground">
          No copy of {activeTab.info.name} is on this machine. Its context is readable here; its
          content is not. Clone the repository and run Kapi in it to reunite the two.
        </div>
        <div className="min-h-0 flex-1">
          <ContextHub
            tabID={tabID}
            projectName={activeTab.info.name}
            section={hubSection}
            contextOnly
          />
        </div>
      </div>
    );
  }

  // Render the active project view, guarded by an error boundary so a single
  // un-openable project (e.g. a recipe whose formats come from a plugin that is
  // not installed — issue #4) shows a recoverable install prompt instead of
  // crashing the webview. The boundary key includes pluginsResolved so a
  // successful install remounts the view.
  // The Project page, also the language view of a project with a target
  // language: the Languages entry opens it on its language standing.
  const homePage = (focusLanguages: boolean) => (
    <HomePage
      project={history.project}
      displayName={activeTab.info.name}
      tabID={tabID}
      onUpdate={updateProject}
      onRunFlow={handleRunFlow}
      onBringUpToDate={handleBringUpToDate}
      onNavigate={navigate}
      onOpenReview={handleOpenReview}
      pluginsResolved={activeTab.pluginsResolved}
      pluginIssues={activeTab.pluginIssues}
      onResetSample={() => onResetSample(tabID)}
      onOpenPoint={handleOpenContext}
      hasTargetLanguages={languages}
      focusLanguages={focusLanguages}
    />
  );
  // The Languages door of a project with no target language. A persisted
  // Review view lands here too, since Review has nothing to hold yet.
  const languagesPage = <LanguagesPage project={history.project} onUpdate={updateProject} />;

  const projectView = ((): ReactNode => {
    switch (route) {
      case "languages":
        return languages ? homePage(true) : languagesPage;

      case "map":
        return (
          <MapPage
            tabID={tabID}
            projectName={activeTab.info.name || history.project.name}
            onDecide={handleDecide}
            onNavigate={navigate}
            onOpenReader={handleOpenReader}
            setupPending={!!activeTab.isEmpty || !!activeTab.detectedPreset}
          />
        );

      case "reader": {
        const target = readerTargetOf(effectiveView);
        if (!target) return null;
        return (
          <ReaderPage
            key={target.path}
            tabID={tabID}
            path={target.path}
            focusBlockID={target.blockID}
            onBack={() => navigate(readerBack)}
            backLabel={readerBack === "checks" ? t("Back to checks") : t("Back to the Map")}
            onNavigate={navigate}
          />
        );
      }

      // `content` folds into the merged project home (issue #1068). Kept as an
      // alias so existing onNavigate("content") calls and saved deep links still
      // resolve to the collection-centric surface.
      case "content":
      case "project-home":
        if (activeTab.isEmpty) {
          return (
            <ProjectSetupPage
              tabID={tabID}
              onDone={() => updateTab(tabID, { isEmpty: false, detectedPreset: undefined })}
            />
          );
        }
        if (activeTab.detectedPreset) {
          return (
            <ProjectPresetPage
              tabID={tabID}
              detectedPreset={activeTab.detectedPreset}
              onApplied={(updated) => {
                history.replace(updated);
                updateTab(tabID, { project: updated, detectedPreset: undefined });
              }}
              onSkip={() => updateTab(tabID, { detectedPreset: undefined })}
            />
          );
        }
        return homePage(false);

      // The Toolbox hosts Tools + Flows as route-driven tabs. Flows are
      // authored/managed here (the library); running a flow against the
      // project's content stays on the Home page.
      case "tools":
      case "flows":
        return (
          <ToolboxPage
            tab={effectiveView === "flows" ? "flows" : "tools"}
            onTabChange={navigate}
            tools={<ToolRunnerPage tabID={tabID} />}
            flows={
              <FlowsPage
                tabID={tabID}
                projectFlows={history.project.flows}
                onRunFlow={handleRunFlow}
                onFlowChange={(name, spec) => {
                  updateProject({
                    ...history.project,
                    flows: { ...history.project.flows, [name]: spec },
                  });
                }}
                onFlowDelete={(name) => {
                  const { [name]: _, ...rest } = history.project.flows ?? {};
                  updateProject({ ...history.project, flows: rest });
                }}
              />
            }
          />
        );

      case "runner":
        if (runnerState) {
          return (
            <RunnerPage
              tabID={tabID}
              flowName={runnerState.flowName}
              flow={runnerState.flow}
              project={history.project}
              autoRun={runnerState.runId !== launchedRunIdRef.current}
              scopePaths={runnerState.scopePaths}
              scopeLabel={runnerState.scopeLabel}
              converge={runnerState.converge}
              onOpenReview={handleOpenReview}
              onLaunched={() => {
                launchedRunIdRef.current = runnerState.runId;
              }}
              onClose={() => {
                setRunnerState(null);
                navigate("project-home");
              }}
            />
          );
        }
        // View-only runner for a job selected from the feed (no runnerState).
        return <RunnerViewFallback tabID={tabID} project={history.project} navigate={navigate} />;

      case "context":
      // The stores are sections of the hub. Their view ids stay routable, so a
      // persisted view still lands where it used to.
      case "termbases":
      case "memories":
        return (
          <ContextHub
            key={contextEntry.current}
            tabID={tabID}
            projectName={history.project.name}
            section={hubSection}
            pin={contextPin ?? undefined}
            decideRow={decideRowOf(effectiveView)}
            hasTargetLanguages={languages}
            contextOnly={activeTab.info.context_only}
            onNavigate={navigate}
          />
        );

      case "checks":
        return (
          <ChecksPanel
            tabID={tabID}
            onOpenContext={handleOpenContext}
            onOpenReader={handleOpenReader}
          />
        );

      case "review":
        if (!languages) return languagesPage;
        return (
          <ReviewPage key={reviewEntry.current} tabID={tabID} scope={reviewScope ?? undefined} />
        );

      // Project Settings holds the project's settings and, under Tools, the
      // app-wide tools, flows, terms, content memories and formats.
      case "project-settings":
        return (
          <ProjectSettingsTabs
            section={settingsSectionOf(effectiveView)}
            onNavigate={navigate}
            settings={
              <ProjectSettingsPage
                tabID={tabID}
                project={history.project}
                onUpdate={updateProject}
                pluginIssues={activeTab.pluginIssues}
                onNavigate={navigate}
              />
            }
            tools={{
              tools: <ToolRunnerPage />,
              flows: <FlowsPage adoptTabID={tabID} adoptProjectName={activeTab.info.name} />,
              terms: <TermsPage />,
              memories: <MemoriesPage />,
              formats: <FormatsPage />,
            }}
          />
        );

      default:
        return null;
    }
  })();

  return (
    <ProjectErrorBoundary
      key={`${tabID}:${effectiveView}:${activeTab.pluginsResolved}`}
      pluginIssues={activeTab.pluginIssues}
      onNavigate={navigate}
    >
      {projectView}
    </ProjectErrorBoundary>
  );
}
