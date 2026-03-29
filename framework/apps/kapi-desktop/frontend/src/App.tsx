import { useState, useCallback } from "react";
import type { View, KapiProject } from "./types/api";
import { WelcomePage } from "./components/WelcomePage";
import { ProjectPage } from "./components/ProjectPage";
import { FlowPage } from "./components/FlowPage";
import { ToolRunnerPage } from "./components/ToolRunnerPage";
import { PluginManager } from "./components/PluginManager";
import { CredentialsPage } from "./components/CredentialsPage";
import { SettingsPage } from "./components/SettingsPage";
import { Sidebar } from "./components/Sidebar";

export default function App() {
  const [view, setView] = useState<View>("welcome");
  const [project, setProject] = useState<KapiProject | null>(null);
  const [projectPath, setProjectPath] = useState<string>("");

  const handleOpenProject = useCallback(
    (proj: KapiProject, path: string) => {
      setProject(proj);
      setProjectPath(path);
      setView("project");
    },
    [],
  );

  const handleNewProject = useCallback((proj: KapiProject) => {
    setProject(proj);
    setProjectPath("");
    setView("project");
  }, []);

  const handleCloseProject = useCallback(() => {
    setProject(null);
    setProjectPath("");
    setView("welcome");
  }, []);

  if (view === "welcome" && !project) {
    return (
      <WelcomePage
        onOpen={handleOpenProject}
        onNew={handleNewProject}
      />
    );
  }

  return (
    <div className="flex h-screen bg-background text-foreground">
      <Sidebar
        activeView={view}
        onViewChange={setView}
        projectName={project?.name}
        onCloseProject={handleCloseProject}
      />
      <main className="flex-1 overflow-auto">
        {view === "project" && project && (
          <ProjectPage
            project={project}
            projectPath={projectPath}
          />
        )}
        {view === "flows" && project && (
          <FlowPage project={project} onUpdate={setProject} />
        )}
        {view === "tools" && <ToolRunnerPage />}
        {view === "plugins" && <PluginManager />}
        {view === "credentials" && <CredentialsPage />}
        {view === "settings" && <SettingsPage />}
      </main>
    </div>
  );
}
