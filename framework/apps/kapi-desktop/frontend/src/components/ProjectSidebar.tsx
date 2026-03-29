import {
  Home,
  FileText,
  Workflow,
  Wrench,
  Plus,
  Trash2,
} from "lucide-react";
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
  projectName,
  flowNames,
  selectedFlow,
  onSelectFlow,
  onAddFlow,
  onDeleteFlow,
}: ProjectSidebarProps) {
  return (
    <aside className="flex w-44 flex-col border-r border-border bg-sidebar">
      {/* Project name */}
      <div className="border-b border-border px-3 py-2">
        <span className="truncate text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {projectName}
        </span>
      </div>

      {/* Navigation */}
      <nav className="space-y-0.5 px-2 pt-2">
        {navItems.map(({ view, label, icon }) => (
          <button
            key={view}
            onClick={() => onViewChange(view)}
            className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs transition-colors ${
              activeView === view
                ? "bg-accent text-accent-foreground font-medium"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
            }`}
          >
            {icon}
            {label}
          </button>
        ))}
      </nav>

      {/* Flow list (when flows view is active) */}
      {activeView === "project-flows" && (
        <div className="mt-2 flex-1 overflow-auto border-t border-border pt-2">
          <div className="flex items-center justify-between px-3 pb-1">
            <span className="text-[10px] font-medium uppercase text-muted-foreground">
              Flows
            </span>
            <button
              onClick={onAddFlow}
              className="rounded p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
              aria-label="New flow"
            >
              <Plus size={10} />
            </button>
          </div>
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
                <button
                  onClick={() => onSelectFlow(name)}
                  className="flex-1 truncate text-left"
                >
                  {name}
                </button>
                <button
                  onClick={() => onDeleteFlow(name)}
                  className="rounded p-0.5 opacity-0 hover:text-destructive group-hover:opacity-100"
                  aria-label={`Delete flow ${name}`}
                >
                  <Trash2 size={10} />
                </button>
              </div>
            ))}
            {flowNames.length === 0 && (
              <p className="px-2 py-1 text-[10px] text-muted-foreground">
                No flows yet
              </p>
            )}
          </div>
        </div>
      )}
    </aside>
  );
}
