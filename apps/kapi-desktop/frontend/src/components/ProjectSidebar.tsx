import { Home, FileText, Workflow, Wrench, Plus, Trash2 } from "lucide-react";
import { Button, ScrollArea } from "@neokapi/ui-primitives";
import type { ProjectView } from "../types/api";

interface ProjectSidebarProps {
  activeView: ProjectView;
  onViewChange: (view: ProjectView) => void;
  projectName: string;
  flowNames: string[];
  selectedFlow: string | null;
  onSelectFlow: (name: string) => void;
  onAddFlow: () => void;
  onDeleteFlow: (name: string) => void;
}

const navItems: { view: ProjectView; label: string; icon: React.ReactNode }[] = [
  { view: "project-home", label: "Home", icon: <Home size={16} /> },
  { view: "content", label: "Content", icon: <FileText size={16} /> },
  { view: "project-flows", label: "Flows", icon: <Workflow size={16} /> },
  { view: "project-tools", label: "Tools", icon: <Wrench size={16} /> },
];

export function ProjectSidebar({
  activeView,
  onViewChange,
  projectName: _projectName,
  flowNames,
  selectedFlow,
  onSelectFlow,
  onAddFlow,
  onDeleteFlow,
}: ProjectSidebarProps) {
  return (
    <aside className="flex w-44 flex-col border-r border-border bg-sidebar">
      {/* Navigation */}
      <nav className="space-y-0.5 px-2 pt-2">
        {navItems.map(({ view, label, icon }) => (
          <Button
            key={view}
            variant="ghost"
            size="sm"
            onClick={() => onViewChange(view)}
            className={`flex w-full justify-start gap-2 text-xs ${
              activeView === view
                ? "bg-accent text-accent-foreground font-medium"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
            }`}
          >
            {icon}
            {label}
          </Button>
        ))}
      </nav>

      {/* Flow list (when flows view is active) */}
      {activeView === "project-flows" && (
        <div className="mt-2 flex-1 border-t border-border pt-2">
          <div className="flex items-center justify-between px-3 pb-1">
            <span className="text-[10px] font-medium uppercase text-muted-foreground">Flows</span>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={onAddFlow}
              className="h-4 w-4"
              aria-label="New flow"
            >
              <Plus size={10} />
            </Button>
          </div>
          <ScrollArea className="flex-1">
            <div className="space-y-0.5 px-2">
              {flowNames.map((name) => (
                <div
                  key={name}
                  className={`group flex items-center gap-1 rounded px-2 py-1 text-xs ${
                    selectedFlow === name
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent/50"
                  }`}
                >
                  <button onClick={() => onSelectFlow(name)} className="flex-1 truncate text-left">
                    {name}
                  </button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => onDeleteFlow(name)}
                    className="h-4 w-4 opacity-0 hover:text-destructive group-hover:opacity-100"
                    aria-label={`Delete flow ${name}`}
                  >
                    <Trash2 size={10} />
                  </Button>
                </div>
              ))}
              {flowNames.length === 0 && (
                <p className="px-2 py-1 text-[10px] text-muted-foreground">No flows yet</p>
              )}
            </div>
          </ScrollArea>
        </div>
      )}
    </aside>
  );
}
