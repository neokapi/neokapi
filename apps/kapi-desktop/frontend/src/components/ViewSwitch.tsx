import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import type { KapiProject, FlowSpec } from "../types/api";
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
import { ContextHub, type ContextPin } from "./ContextHub";
import { ReviewPage, type ReviewScope } from "./ReviewPage";
import { FormatsPage } from "./FormatsPage";
import { SettingsPage } from "./SettingsPage";
import { HomePage } from "./HomePage";
import { ProjectSetupPage } from "./ProjectSetupPage";
import { ProjectSettingsPage } from "./ProjectSettingsPage";
import { ProjectPresetPage } from "./ProjectPresetPage";
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
  mode: "adhoc" | "projects";
  effectiveView: string;
  activeTab: TabState | null;
  history: ProjectHistory;
  updateProject: (project: KapiProject) => void;
  navigate: (view: string) => void;
  updateTab: (id: string, patch: Partial<TabState>) => void;
  // Home page props
  recentFiles: Array<{ path: string; name: string; opened_at: string }>;
  samplesDismissed: boolean;
  onOpenRecent: (path: string) => void;
  onNewProject: () => void;
  onOpenProject: () => void;
  onCreateSampleProject: (name: string) => void;
  onDismissSamples: () => void;
  /** Reset an out-of-date sample project to the version bundled with this kapi. */
  onResetSample: (tabID: string) => void;
  /** An open project tab to offer ad-hoc flow adoption into ({id,name}). */
  adoptTarget?: { id: string; name: string } | null;
}

export function ViewSwitch({
  mode,
  effectiveView,
  activeTab,
  history,
  updateProject,
  navigate,
  updateTab,
  recentFiles,
  samplesDismissed,
  onOpenRecent,
  onNewProject,
  onOpenProject,
  onCreateSampleProject,
  onDismissSamples,
  onResetSample,
  adoptTarget,
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

  // Home — global overlay in both modes
  if (effectiveView === "home") {
    return (
      <AppHome
        recentFiles={recentFiles}
        samplesDismissed={samplesDismissed}
        onOpenRecent={onOpenRecent}
        onNewProject={onNewProject}
        onOpenProject={onOpenProject}
        onNavigate={navigate}
        onCreateSampleProject={onCreateSampleProject}
        onDismissSamples={onDismissSamples}
      />
    );
  }

  // Ad-hoc views
  if (mode === "adhoc") {
    switch (effectiveView) {
      // The Toolbox hosts Tools + Flows as route-driven tabs; flows are no
      // longer a sidebar pillar.
      case "tools":
      case "flows":
        return (
          <ToolboxPage
            tab={effectiveView === "flows" ? "flows" : "tools"}
            onTabChange={navigate}
            tools={<ToolRunnerPage />}
            flows={<FlowsPage adoptTabID={adoptTarget?.id} adoptProjectName={adoptTarget?.name} />}
          />
        );
      case "termbases":
        return <TermsPage />;
      case "memories":
        return <MemoriesPage />;
      case "formats":
        return <FormatsPage />;
      case "settings":
      case "app-settings":
        return <SettingsPage />;
      default:
        return null;
    }
  }

  // Global app settings — accessible from any mode via bottom gear icon
  if (effectiveView === "app-settings") {
    return <SettingsPage />;
  }

  // Project views — require an active tab
  if (!activeTab) return null;

  const tabID = activeTab.info.id;

  // Render the active project view, guarded by an error boundary so a single
  // un-openable project (e.g. a recipe whose formats come from a plugin that is
  // not installed — issue #4) shows a recoverable install prompt instead of
  // crashing the webview. The boundary key includes pluginsResolved so a
  // successful install remounts the view.
  const projectView = ((): ReactNode => {
    switch (effectiveView) {
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
        return (
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
          />
        );

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
            section={
              effectiveView === "termbases"
                ? "terms"
                : effectiveView === "memories"
                  ? "memory"
                  : undefined
            }
            pin={contextPin ?? undefined}
            hasTargetLanguages={(history.project.defaults?.target_languages?.length ?? 0) > 0}
          />
        );

      case "checks":
        return <ChecksPanel tabID={tabID} onOpenContext={handleOpenContext} />;

      case "review":
        return (
          <ReviewPage key={reviewEntry.current} tabID={tabID} scope={reviewScope ?? undefined} />
        );

      case "project-settings":
        return (
          <ProjectSettingsPage
            tabID={tabID}
            project={history.project}
            onUpdate={updateProject}
            pluginIssues={activeTab.pluginIssues}
            onNavigate={navigate}
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
