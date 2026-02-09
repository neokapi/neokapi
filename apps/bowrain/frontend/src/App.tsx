import { useState, useCallback, useEffect } from "react";
import { Sidebar, type View } from "./components/Sidebar";
import { Header } from "./components/Header";
import { SettingsPage } from "./components/SettingsPage";
import { ConvertPanel } from "./components/ConvertPanel";
import { TranslatePanel } from "./components/TranslatePanel";
import { ProjectDashboard } from "./components/ProjectDashboard";
import { ProjectView } from "./components/ProjectView";
import { TranslationEditor } from "./components/TranslationEditor";
import { TMExplorer } from "./components/TMExplorer";
import { TermExplorer } from "./components/TermExplorer";
import { FlowBuilder } from "./components/FlowBuilder";
import { useHealth, useProjectApi } from "./hooks/useApi";
import type { ProjectInfo } from "./types/api";
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../bindings/github.com/gokapi/gokapi/apps/bowrain/backend/app.js";

function App() {
  const [activeView, setActiveView] = useState<View>("projects");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const { connected } = useHealth();

  // Project state
  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [activeProject, setActiveProject] = useState<ProjectInfo | null>(null);
  const [activeFile, setActiveFile] = useState<string | null>(null);
  const [showTMExplorer, setShowTMExplorer] = useState(false);
  const [showTermExplorer, setShowTermExplorer] = useState(false);

  const projectApi = useProjectApi();

  // Auto-open a project if a .kaz path was passed via CLI args.
  useEffect(() => {
    Backend.GetInitialProject().then((path: string) => {
      if (!path) return;
      projectApi.openProject(path).then((info) => {
        setProjects((prev) => [...prev, info]);
        setActiveProject(info);
        setActiveView("projects");
      }).catch((e: unknown) => {
        console.error("Failed to open initial project:", e);
      });
    });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

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
        const fresh = await Backend.GetProject(project.id) as ProjectInfo;
        setActiveProject(fresh);
        setProjects((prev) => prev.map((p) => (p.id === fresh.id ? fresh : p)));
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
    setShowTMExplorer(false);
    setShowTermExplorer(false);
  }, []);

  const handleBackToProject = useCallback(() => {
    setActiveFile(null);
    setShowTMExplorer(false);
    setShowTermExplorer(false);
  }, []);

  const handleOpenTM = useCallback(() => {
    setShowTMExplorer(true);
    setShowTermExplorer(false);
  }, []);

  const handleOpenTerms = useCallback(() => {
    setShowTermExplorer(true);
    setShowTMExplorer(false);
  }, []);

  const handleViewChange = useCallback((view: View) => {
    setActiveView(view);
    if (view !== "projects") {
      setActiveProject(null);
      setActiveFile(null);
      setShowTMExplorer(false);
    }
  }, []);

  const renderView = () => {
    // If we're in the projects view and have Term explorer open, show it
    if (activeView === "projects" && activeProject && showTermExplorer) {
      return (
        <TermExplorer
          project={activeProject}
          onBack={handleBackToProject}
        />
      );
    }

    // If we're in the projects view and have TM explorer open, show it
    if (activeView === "projects" && activeProject && showTMExplorer) {
      return (
        <TMExplorer
          project={activeProject}
          onBack={handleBackToProject}
        />
      );
    }

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
          onOpenTM={handleOpenTM}
          onOpenTerms={handleOpenTerms}
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
      case "flows":
        return <FlowBuilder />;
    }
  };

  const isEditor = activeView === "projects" && activeProject != null && activeFile != null;
  const isFlowBuilder = activeView === "flows";

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
            overflow: isEditor || isFlowBuilder ? "hidden" : "auto",
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
