import type { KapiProject } from "../types/api";
import type { TabState } from "../hooks/useTabManager";
import type { ProjectHistory } from "../hooks/useProjectHistory";
import { AppHome } from "./AppHome";
import { FlowsPage } from "./FlowsPage";
import { ToolRunnerPage } from "./ToolRunnerPage";
import { TermbasesPage } from "./TermbasesPage";
import { MemoriesPage } from "./MemoriesPage";
import { FormatsPage } from "./FormatsPage";
import { SettingsPage } from "./SettingsPage";
import { HomePage } from "./HomePage";
import { ContentPage } from "./ContentPage";
import { ProjectSetupPage } from "./ProjectSetupPage";
import { ProjectSettingsPage } from "./ProjectSettingsPage";
import { ProjectPresetPage } from "./ProjectPresetPage";

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
}: ViewSwitchProps) {
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
        return <FlowsPage />;
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
          onNavigate={navigate}
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

    case "tools":
      return <ToolRunnerPage />;

    case "termbases":
      return <TermbasesPage />;

    case "memories":
      return <MemoriesPage />;

    case "project-settings":
      return <ProjectSettingsPage project={history.project} onUpdate={updateProject} />;

    default:
      return null;
  }
}
