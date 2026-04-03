import { FolderKanban, FolderOpen, Plus } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";
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
          <Button
            size="sm"
            onClick={onNewProject}
          >
            <Plus size={12} />
            New Project
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={onOpenProject}
          >
            <FolderOpen size={12} />
            Open
          </Button>
        </div>
      </div>

      {tabs.length > 0 ? (
        <div className="space-y-2">
          {tabs.map((tab) => (
            <Button
              key={tab.id}
              variant="outline"
              onClick={() => onSelectTab(tab.id)}
              className="flex w-full h-auto items-center gap-3 rounded-lg p-4 text-left hover:bg-accent/30"
            >
              <FolderKanban size={20} className="shrink-0 text-primary" />
              <div className="flex-1">
                <div className="text-sm font-medium">{tab.name}</div>
                {tab.path && (
                  <div className="truncate text-xs text-muted-foreground">{tab.path}</div>
                )}
              </div>
            </Button>
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
