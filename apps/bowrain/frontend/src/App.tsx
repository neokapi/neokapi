import { useState, useCallback, useEffect, useMemo } from "react";
import { Sidebar, type View } from "./components/Sidebar";
import { Header } from "./components/Header";
import { SettingsPage } from "./components/SettingsPage";
import {
  ApiProvider,
  WorkspaceProvider,
  ProjectDashboard,
  ProjectView,
  TranslationEditor,
  TMExplorer,
  TermExplorer,
} from "@gokapi/ui";
import { FlowBuilder } from "./components/FlowBuilder";
import { ConnectorPanel } from "./components/ConnectorPanel";
import { DocumentPreview } from "./components/DocumentPreview";
import { useHealth } from "./hooks/useApi";
import { WailsApiAdapter } from "./api/WailsApiAdapter";
import type { ProjectInfo, BlockInfo } from "@gokapi/ui";

const wailsAdapter = new WailsApiAdapter();
const localWorkspace = { id: "local", name: "Personal", slug: "personal", description: "", logo_url: "", role: "owner" as const };

function App() {
  const [activeView, setActiveView] = useState<View>("translate");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const { connected } = useHealth();

  // Project state
  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [activeProject, setActiveProject] = useState<ProjectInfo | null>(null);
  const [activeFile, setActiveFile] = useState<string | null>(null);
  const [showTMExplorer, setShowTMExplorer] = useState(false);
  const [showTermExplorer, setShowTermExplorer] = useState(false);

  // Auto-open a project if a .kaz path was passed via CLI args.
  useEffect(() => {
    wailsAdapter.getInitialProject().then((path: string) => {
      if (!path) return;
      wailsAdapter.getProject("personal", path).then((info) => {
        setProjects((prev) => [...prev, info]);
        setActiveProject(info);
        setActiveView("translate");
      }).catch((e: unknown) => {
        console.error("Failed to open initial project:", e);
      });
    });
  }, []);

  const handleCreateProject = useCallback(
    async (name: string, sourceLang: string, targetLangs: string[]) => {
      try {
        const info = await wailsAdapter.createProject("personal", name, sourceLang, targetLangs);
        setProjects((prev) => [...prev, info]);
        setActiveProject(info);
      } catch (e) {
        console.error("Create project failed:", e);
      }
    },
    [],
  );

  const handleOpenProject = useCallback(
    async (project: ProjectInfo) => {
      try {
        const fresh = await wailsAdapter.getProject("personal", project.id);
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
      const info = await wailsAdapter.openProjectDialog();
      if (!info) return;
      setProjects((prev) => [...prev, info]);
      setActiveProject(info);
    } catch (e) {
      console.error("Open project failed:", e);
    }
  }, []);

  const handleUploadFiles = useCallback(
    async (files: File[]) => {
      if (!activeProject) return;
      try {
        const updated = await wailsAdapter.uploadFiles("personal", activeProject.id, files);
        setActiveProject(updated);
        setProjects((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
      } catch (e) {
        console.error("Add files failed:", e);
      }
    },
    [activeProject],
  );

  const handleRemoveFile = useCallback(
    async (fileName: string) => {
      if (!activeProject) return;
      try {
        const updated = await wailsAdapter.removeFile("personal", activeProject.id, fileName);
        setActiveProject(updated);
        setProjects((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
      } catch (e) {
        console.error("Remove file failed:", e);
      }
    },
    [activeProject],
  );

  const handleSaveProject = useCallback(async () => {
    if (!activeProject) return;
    try {
      if ((activeProject as ProjectInfo & { path?: string }).path) {
        await wailsAdapter.saveProject(activeProject.id);
      } else {
        await wailsAdapter.saveProjectDialog(activeProject.id);
      }
    } catch (e) {
      console.error("Save project failed:", e);
    }
  }, [activeProject]);

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
    if (view !== "translate") {
      setActiveProject(null);
      setActiveFile(null);
      setShowTMExplorer(false);
    }
  }, []);

  // Export handler for desktop: the WailsApiAdapter already handles export + open
  const handleDesktopExport = useCallback((_blob: Blob, _fileName: string) => {
    // No-op: WailsApiAdapter.exportTranslatedFile already exported to disk and opened in OS
  }, []);

  // Render preview for split layout modes
  const renderDesktopPreview = useMemo(() => {
    return (props: {
      projectId: string;
      itemName: string;
      targetLocale: string;
      selectedBlockId?: string;
      onBlockSelect: (blockId: string) => void;
      blocks: BlockInfo[];
    }) => (
      <DocumentPreview
        projectId={props.projectId}
        itemName={props.itemName}
        targetLocale={props.targetLocale}
        selectedBlockId={props.selectedBlockId}
        onBlockSelect={props.onBlockSelect}
        blocks={props.blocks}
      />
    );
  }, []);

  const renderView = () => {
    if (activeView === "translate" && activeProject && showTermExplorer) {
      return (
        <TermExplorer
          sourceLocale={activeProject.source_locale}
          targetLocales={activeProject.target_locales}
          projectName={activeProject.name}
          onBack={handleBackToProject}
        />
      );
    }

    if (activeView === "translate" && activeProject && showTMExplorer) {
      return (
        <TMExplorer
          sourceLocale={activeProject.source_locale}
          targetLocales={activeProject.target_locales}
          onBack={handleBackToProject}
        />
      );
    }

    if (activeView === "translate" && activeProject && activeFile) {
      return (
        <TranslationEditor
          project={activeProject}
          fileName={activeFile}
          onBack={handleBackToProject}
          onExport={handleDesktopExport}
          renderPreview={renderDesktopPreview}
        />
      );
    }

    if (activeView === "translate" && activeProject) {
      return (
        <ProjectView
          project={activeProject}
          onBack={handleBackToProjects}
          onOpenFile={handleOpenFile}
          onUploadFiles={handleUploadFiles}
          onRemoveFile={handleRemoveFile}
          onSave={handleSaveProject}
          onOpenTM={handleOpenTM}
          onOpenTerms={handleOpenTerms}
        />
      );
    }

    switch (activeView) {
      case "translate":
        return (
          <ProjectDashboard
            projects={projects}
            onCreateProject={handleCreateProject}
            onOpenProject={handleOpenProject}
            onOpenKaz={handleOpenKaz}
          />
        );
      case "termbase":
        return <div style={{ color: "var(--text-secondary)", padding: 24 }}>Select a project to explore its termbase.</div>;
      case "memory":
        return <div style={{ color: "var(--text-secondary)", padding: 24 }}>Select a project to explore its translation memory.</div>;
      case "settings":
        return <SettingsPage />;
      case "flows":
        return <FlowBuilder />;
      case "connectors":
        return <ConnectorPanel />;
    }
  };

  const isEditor = activeView === "translate" && activeProject != null && activeFile != null;
  const isFlowBuilder = activeView === "flows";

  return (
    <ApiProvider adapter={wailsAdapter}>
      <WorkspaceProvider initialWorkspace={localWorkspace}>
        <div
          style={{
            display: "flex",
            height: "100vh",
            overflow: "hidden",
          }}
        >
          <Sidebar
            activeView={activeView}
            onViewChange={handleViewChange}
            collapsed={sidebarCollapsed}
            onCollapsedChange={setSidebarCollapsed}
            workspaceName="Personal"
          />
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
      </WorkspaceProvider>
    </ApiProvider>
  );
}

export default App;
