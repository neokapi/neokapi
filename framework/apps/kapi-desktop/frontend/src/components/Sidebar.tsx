import { Home, FileText, Workflow, Wrench, Plus, Trash2 } from "lucide-react";
import type { View } from "../types/api";

interface SidebarProps {
  activeView: View;
  onViewChange: (view: View) => void;
  flowNames?: string[];
  selectedFlow?: string | null;
  onSelectFlow?: (name: string) => void;
  onAddFlow?: () => void;
  onDeleteFlow?: (name: string) => void;
}

const navItems: { view: View; label: string; icon: React.ReactNode }[] = [
  { view: "home", label: "Home", icon: <Home size={18} /> },
  { view: "content", label: "Content", icon: <FileText size={18} /> },
  { view: "flows", label: "Flows", icon: <Workflow size={18} /> },
  { view: "tools", label: "Tools", icon: <Wrench size={18} /> },
];

export function Sidebar({
  activeView,
  onViewChange,
  flowNames = [],
  selectedFlow,
  onSelectFlow,
  onAddFlow,
  onDeleteFlow,
}: SidebarProps) {
  return (
    <aside className="flex w-52 flex-col border-r border-border bg-sidebar pt-2">
      {/* Main navigation */}
      <nav className="space-y-0.5 px-2">
        {navItems.map(({ view, label, icon }) => (
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
      </nav>

      {/* Flow list (shown when Flows view is active) */}
      {activeView === "flows" && (
        <div className="mt-2 flex-1 overflow-auto border-t border-border pt-2">
          <div className="flex items-center justify-between px-3 pb-1">
            <span className="text-xs font-medium text-muted-foreground">Flows</span>
            {onAddFlow && (
              <button
                onClick={onAddFlow}
                className="rounded p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                aria-label="New flow"
              >
                <Plus size={12} />
              </button>
            )}
          </div>
          <div className="space-y-0.5 px-2">
            {flowNames.map((name) => (
              <div
                key={name}
                className={`group flex items-center gap-1 rounded px-2 py-1 text-sm ${
                  selectedFlow === name
                    ? "bg-accent text-accent-foreground"
                    : "text-muted-foreground hover:bg-accent/50"
                }`}
              >
                <button
                  onClick={() => onSelectFlow?.(name)}
                  className="flex-1 truncate text-left text-xs"
                >
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
              <p className="px-2 py-1 text-xs text-muted-foreground">No flows yet</p>
            )}
          </div>
        </div>
      )}
    </aside>
  );
}
