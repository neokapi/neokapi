import { useState, useCallback } from "react";
import { Sidebar, type View } from "./components/Sidebar";
import { Header } from "./components/Header";
import { SettingsPage } from "./components/SettingsPage";
import { ConvertPanel } from "./components/ConvertPanel";
import { TranslatePanel } from "./components/TranslatePanel";
import { ProjectDashboard } from "./components/ProjectDashboard";
import { ProjectView } from "./components/ProjectView";
import { TranslationEditor } from "./components/TranslationEditor";
import { useHealth, useProjectApi } from "./hooks/useApi";
import type { ProjectInfo } from "./types/api";

function App() {
  const [activeView, setActiveView] = useState<View>("projects");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const { connected } = useHealth();

  // Project state
  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [activeProject, setActiveProject] = useState<ProjectInfo | null>(null);
  const [activeFile, setActiveFile] = useState<string | null>(null);

  const projectApi = useProjectApi();

  const handleCreateProject = useCallback(
    async (name: string, sourceLang: string, targetLangs: string[]) => {
      try {
        const info = await projectApi.createProject(name, sourceLang, targetLangs);
        setProjects((prev) => [...prev, info]);
        setActiveProject(info);
      } catch (e) {
        console.error("Create project failed:", e);
      }
    },
    [projectApi],
  );

  const handleOpenProject = useCallback(
    async (project: ProjectInfo) => {
      // Fetch fresh data from backend (in case files were added externally)
      try {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const w = window as any;
        if (w?.go?.backend?.App?.GetProject) {
          const fresh = await w.go.backend.App.GetProject(project.id);
          setActiveProject(fresh);
          setProjects((prev) => prev.map((p) => (p.id === fresh.id ? fresh : p)));
        } else {
          setActiveProject(project);
        }
      } catch {
        setActiveProject(project);
      }
      setActiveFile(null);
    },
    [],
  );

  const handleOpenKaz = useCallback(async () => {
    try {
      const info = await projectApi.openProjectDialog();
      if (!info) return; // user cancelled
      setProjects((prev) => [...prev, info]);
      setActiveProject(info);
    } catch (e) {
      console.error("Open project failed:", e);
    }
  }, [projectApi]);

  const handleAddFiles = useCallback(
    async (filePaths: string[]) => {
      if (!activeProject) return;
      try {
        const updated = await projectApi.addFiles(activeProject.id, filePaths);
        setActiveProject(updated);
        setProjects((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
      } catch (e) {
        console.error("Add files failed:", e);
      }
    },
    [activeProject, projectApi],
  );

  const handleAddFilesDialog = useCallback(async () => {
    if (!activeProject) return;
    try {
      const updated = await projectApi.addFilesDialog(activeProject.id);
      if (!updated) return; // user cancelled
      setActiveProject(updated);
      setProjects((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
    } catch (e) {
      console.error("Add files failed:", e);
    }
  }, [activeProject, projectApi]);

  const handleRemoveFile = useCallback(
    async (fileName: string) => {
      if (!activeProject) return;
      try {
        const updated = await projectApi.removeFile(activeProject.id, fileName);
        setActiveProject(updated);
        setProjects((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
      } catch (e) {
        console.error("Remove file failed:", e);
      }
    },
    [activeProject, projectApi],
  );

  const handleSaveProject = useCallback(async () => {
    if (!activeProject) return;
    try {
      if (activeProject.path) {
        await projectApi.saveProject(activeProject.id);
      } else {
        await projectApi.saveProjectDialog(activeProject.id);
      }
    } catch (e) {
      console.error("Save project failed:", e);
    }
  }, [activeProject, projectApi]);

  const handleOpenFile = useCallback((fileName: string) => {
    setActiveFile(fileName);
  }, []);

  const handleBackToProjects = useCallback(() => {
    setActiveProject(null);
    setActiveFile(null);
  }, []);

  const handleBackToProject = useCallback(() => {
    setActiveFile(null);
  }, []);

  const handleViewChange = useCallback((view: View) => {
    setActiveView(view);
    if (view !== "projects") {
      setActiveProject(null);
      setActiveFile(null);
    }
  }, []);

  const renderView = () => {
    // If we're in the projects view and have an active file, show editor
    if (activeView === "projects" && activeProject && activeFile) {
      return (
        <TranslationEditor
          project={activeProject}
          fileName={activeFile}
          onBack={handleBackToProject}
        />
      );
    }

    // If we're in the projects view and have an active project, show project view
    if (activeView === "projects" && activeProject) {
      return (
        <ProjectView
          project={activeProject}
          onBack={handleBackToProjects}
          onOpenFile={handleOpenFile}
          onAddFiles={handleAddFiles}
          onAddFilesDialog={handleAddFilesDialog}
          onRemoveFile={handleRemoveFile}
          onSave={handleSaveProject}
        />
      );
    }

    switch (activeView) {
      case "projects":
        return (
          <ProjectDashboard
            projects={projects}
            onCreateProject={handleCreateProject}
            onOpenProject={handleOpenProject}
            onOpenKaz={handleOpenKaz}
          />
        );
      case "settings":
        return <SettingsPage />;
      case "convert":
        return <ConvertPanel />;
      case "translate":
        return <TranslatePanel />;
    }
  };

  const isEditor = activeView === "projects" && activeProject != null && activeFile != null;

  return (
    <div
      style={{
        display: "flex",
        height: "100vh",
        overflow: "hidden",
      }}
    >
      <Sidebar activeView={activeView} onViewChange={handleViewChange} collapsed={sidebarCollapsed} onCollapsedChange={setSidebarCollapsed} />
      <div style={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }}>
        <Header connected={connected} sidebarCollapsed={sidebarCollapsed} />
        <main
          style={{
            flex: 1,
            padding: 24,
            overflow: isEditor ? "hidden" : "auto",
            display: "flex",
            flexDirection: "column",
            minHeight: 0,
          }}
        >
          {renderView()}
        </main>
      </div>
    </div>
  );
}

export default App;
