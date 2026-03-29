import { useState, useCallback, useEffect } from "react";
import type { View, KapiProject } from "./types/api";
import { api } from "./hooks/useApi";
import { WelcomePage } from "./components/WelcomePage";
import { ProjectPage } from "./components/ProjectPage";
import { FlowPage } from "./components/FlowPage";
import { ToolRunnerPage } from "./components/ToolRunnerPage";
import { PluginManager } from "./components/PluginManager";
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

  // Listen for native menu events from the Go backend.
  useEffect(() => {
    let cleanups: Array<() => void> = [];

    import("@wailsio/runtime")
      .then(({ Events }) => {
        cleanups.push(
          Events.On("menu:new-project", () => {
            handleNewProject({
              version: "v1",
              name: "New Project",
              source_language: "en-US",
              target_languages: [],
              flows: {},
            });
          }),
        );

        cleanups.push(
          Events.On("menu:open-project", async () => {
            const result = await api.openProjectDialog();
            if (result) {
              const path = (await api.getProjectPath()) ?? "";
              handleOpenProject(result, path);
            }
          }),
        );

        cleanups.push(
          Events.On("menu:save-project", async () => {
            if (projectPath) {
              await api.saveProject();
            } else {
              await api.saveProjectDialog();
              const path = await api.getProjectPath();
              if (path) setProjectPath(path);
            }
          }),
        );

        cleanups.push(
          Events.On("menu:save-project-as", async () => {
            await api.saveProjectDialog();
            const path = await api.getProjectPath();
            if (path) setProjectPath(path);
          }),
        );
      })
      .catch(() => {
        // Not in Wails runtime (Storybook, tests).
      });

    return () => {
      cleanups.forEach((fn) => fn());
    };
  }, [handleNewProject, handleOpenProject, projectPath]);

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
            onSaved={setProjectPath}
          />
        )}
        {view === "flows" && project && (
          <FlowPage project={project} onUpdate={setProject} />
        )}
        {view === "tools" && <ToolRunnerPage />}
        {view === "plugins" && <PluginManager />}
        {view === "settings" && <SettingsPage />}
      </main>
    </div>
  );
}
