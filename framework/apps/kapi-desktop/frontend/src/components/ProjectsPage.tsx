import { FolderKanban, FolderOpen, Plus } from "lucide-react";
import { Button, PageHeader, EmptyState } from "@neokapi/ui-primitives";
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
      <PageHeader
        title="Projects"
        actions={
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
        }
      />

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
        <EmptyState
          icon={<FolderKanban size={24} className="text-muted-foreground/50" />}
          title="No projects open. Create a new project or open an existing one."
        />
      )}
    </div>
  );
}
