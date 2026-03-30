import { FolderKanban, FolderOpen, Plus } from "lucide-react";
import type { TabInfo } from "../types/api";

interface ProjectsPageProps {
  tabs: TabInfo[];
  onSelectTab: (id: string) => void;
  onNewProject: () => void;
  onOpenProject: () => void;
}

export function ProjectsPage({
  tabs,
  onSelectTab,
  onNewProject,
  onOpenProject,
}: ProjectsPageProps) {
  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Projects</h1>
        <div className="flex gap-2">
          <button
            onClick={onNewProject}
            className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          >
            <Plus size={12} />
            New Project
          </button>
          <button
            onClick={onOpenProject}
            className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent"
          >
            <FolderOpen size={12} />
            Open
          </button>
        </div>
      </div>

      {tabs.length > 0 ? (
        <div className="space-y-2">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => onSelectTab(tab.id)}
              className="flex w-full items-center gap-3 rounded-lg border border-border p-4 text-left transition-colors hover:bg-accent/30"
            >
              <FolderKanban size={20} className="shrink-0 text-primary" />
              <div className="flex-1">
                <div className="text-sm font-medium">{tab.name}</div>
                {tab.path && (
                  <div className="truncate text-xs text-muted-foreground">{tab.path}</div>
                )}
              </div>
            </button>
          ))}
        </div>
      ) : (
        <div className="rounded-lg border border-dashed border-border p-8 text-center">
          <FolderKanban size={24} className="mx-auto mb-2 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">
            No projects open. Create a new project or open an existing one.
          </p>
        </div>
      )}
    </div>
  );
}
