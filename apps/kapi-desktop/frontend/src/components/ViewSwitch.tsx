import { useCallback, useEffect, useState, type ReactNode } from "react";
import type { KapiProject, FlowSpec } from "../types/api";
import { ProjectErrorBoundary } from "./ProjectErrorBoundary";
import type { TabState } from "../hooks/useTabManager";
import type { ProjectHistory } from "../hooks/useProjectHistory";
import { AppHome } from "./AppHome";
import { RunnerPage } from "./RunnerPage";
import { FlowsPage } from "./FlowsPage";
import { ToolRunnerPage } from "./ToolRunnerPage";
import { TermbasesPage } from "./TermbasesPage";
import { MemoriesPage } from "./MemoriesPage";
import { ChecksPanel } from "./ChecksPanel";
import { FormatsPage } from "./FormatsPage";
import { SettingsPage } from "./SettingsPage";
import { HomePage } from "./HomePage";
import { ContentPage } from "./ContentPage";
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
  adoptTarget,
}: ViewSwitchProps) {
  // State for the runner view — set when the user clicks Run on a flow.
  const [runnerState, setRunnerState] = useState<{
    flowName: string;
    flow: FlowSpec;
  } | null>(null);

  // Run a project flow from the home page: navigate to the runner view.
  const handleRunFlow = useCallback(
    (_flowName: string, spec: FlowSpec) => {
      setRunnerState({ flowName: _flowName, flow: spec });
      navigate("runner");
    },
    [navigate],
  );

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
      case "flows":
        return <FlowsPage adoptTabID={adoptTarget?.id} adoptProjectName={adoptTarget?.name} />;
      case "tools":
        return <ToolRunnerPage />;
      case "termbases":
        return <TermbasesPage />;
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
  // un-openable project (e.g. OkapiMart without okapi-bridge — issue #4) shows
  // a recoverable install prompt instead of crashing the webview. The boundary
  // key includes pluginsResolved so a successful install remounts the view.
  const projectView = ((): ReactNode => {
    switch (effectiveView) {
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
            onRunFlow={handleRunFlow}
            onNavigate={navigate}
            pluginsResolved={activeTab.pluginsResolved}
            pluginIssues={activeTab.pluginIssues}
          />
        );

      case "content":
        return (
          <ContentPage
            project={history.project}
            projectPath={activeTab.info.path}
            onUpdate={updateProject}
            tabID={tabID}
          />
        );

      case "flows":
        return (
          <FlowsPage
            tabID={tabID}
            projectFlows={history.project.flows}
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
        );

      case "runner":
        if (runnerState) {
          return (
            <RunnerPage
              tabID={tabID}
              flowName={runnerState.flowName}
              flow={runnerState.flow}
              project={history.project}
              autoRun
              onClose={() => {
                setRunnerState(null);
                navigate("project-home");
              }}
            />
          );
        }
        // View-only runner for a job selected from the feed (no runnerState).
        return <RunnerViewFallback tabID={tabID} project={history.project} navigate={navigate} />;

      case "tools":
        return <ToolRunnerPage tabID={tabID} />;

      case "checks":
        return <ChecksPanel tabID={tabID} />;

      case "termbases":
        return <TermbasesPage tabID={tabID} />;

      case "memories":
        return <MemoriesPage tabID={tabID} />;

      case "project-settings":
        return (
          <ProjectSettingsPage
            project={history.project}
            onUpdate={updateProject}
            pluginIssues={activeTab.pluginIssues}
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
