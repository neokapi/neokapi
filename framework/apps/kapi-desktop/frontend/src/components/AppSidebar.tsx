import {
  Home,
  Workflow,
  Wrench,
  BookOpen,
  Database,
  FileText,
  FolderKanban,
  Settings,
  Plus,
  Trash2,
} from "lucide-react";
import type { AdhocView, ProjectView } from "../types/api";

// --- Ad-Hoc mode sidebar ---

const adhocItems: { view: AdhocView; label: string; icon: React.ReactNode }[] = [
  { view: "home", label: "Home", icon: <Home size={18} strokeWidth={1.5} /> },
  { view: "flows", label: "Flows", icon: <Workflow size={18} strokeWidth={1.5} /> },
  { view: "tools", label: "Tools", icon: <Wrench size={18} strokeWidth={1.5} /> },
  { view: "termbases", label: "Termbases", icon: <BookOpen size={18} strokeWidth={1.5} /> },
  { view: "memories", label: "TM", icon: <Database size={18} strokeWidth={1.5} /> },
  { view: "formats", label: "Formats", icon: <FileText size={18} strokeWidth={1.5} /> },
];

const projectItems: { view: ProjectView; label: string; icon: React.ReactNode }[] = [
  { view: "home", label: "Home", icon: <FolderKanban size={18} strokeWidth={1.5} /> },
  { view: "content", label: "Content", icon: <FileText size={18} strokeWidth={1.5} /> },
  { view: "flows", label: "Flows", icon: <Workflow size={18} strokeWidth={1.5} /> },
  { view: "tools", label: "Tools", icon: <Wrench size={18} strokeWidth={1.5} /> },
];

interface AppSidebarProps {
  mode: "adhoc" | "projects";
  activeView: string;
  onViewChange: (view: string) => void;
  // Flow list (projects mode only)
  flowNames?: string[];
  selectedFlow?: string | null;
  onSelectFlow?: (name: string) => void;
  onAddFlow?: () => void;
  onDeleteFlow?: (name: string) => void;
}

export function AppSidebar({
  mode,
  activeView,
  onViewChange,
  flowNames = [],
  selectedFlow,
  onSelectFlow,
  onAddFlow,
  onDeleteFlow,
}: AppSidebarProps) {
  const items = mode === "adhoc" ? adhocItems : projectItems;
  const isFlowsActive = activeView === "flows";

  return (
    <aside className="flex w-48 flex-col border-r border-border bg-sidebar">
      {/* Navigation */}
      <nav className="flex-1 space-y-0.5 px-2 pt-2">
        {items.map(({ view, label, icon }) => (
          <button
            key={view}
            onClick={() => onViewChange(view)}
            className={`flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm transition-colors ${
              activeView === view
                ? "bg-accent text-accent-foreground font-medium"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
            }`}
          >
            {icon}
            {label}
          </button>
        ))}

        {/* Flow list inline when Flows is active in projects mode */}
        {mode === "projects" && isFlowsActive && (
          <div className="ml-2 mt-1 border-l border-border pl-2">
            <div className="flex items-center justify-between py-1">
              <span className="text-[10px] font-medium uppercase text-muted-foreground">
                Project Flows
              </span>
              {onAddFlow && (
                <button
                  onClick={onAddFlow}
                  className="rounded p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                  aria-label="New flow"
                >
                  <Plus size={10} />
                </button>
              )}
            </div>
            {flowNames.map((name) => (
              <div
                key={name}
                className={`group flex items-center gap-1 rounded px-2 py-1 text-xs ${
                  selectedFlow === name
                    ? "bg-accent text-accent-foreground"
                    : "text-muted-foreground hover:bg-accent/50"
                }`}
              >
                <button onClick={() => onSelectFlow?.(name)} className="flex-1 truncate text-left">
                  {name}
                </button>
                {onDeleteFlow && (
                  <button
                    onClick={() => onDeleteFlow(name)}
                    className="rounded p-0.5 opacity-0 hover:text-destructive group-hover:opacity-100"
                    aria-label={`Delete flow ${name}`}
                  >
                    <Trash2 size={10} />
                  </button>
                )}
              </div>
            ))}
            {flowNames.length === 0 && (
              <p className="px-2 py-1 text-[10px] text-muted-foreground">No flows yet</p>
            )}
          </div>
        )}
      </nav>

      {/* Settings at bottom */}
      <div className="border-t border-border px-2 py-2">
        <button
          onClick={() => onViewChange("settings")}
          className={`flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm transition-colors ${
            activeView === "settings"
              ? "bg-accent text-accent-foreground font-medium"
              : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
          }`}
        >
          <Settings size={18} strokeWidth={1.5} />
          Settings
        </button>
      </div>
    </aside>
  );
}
