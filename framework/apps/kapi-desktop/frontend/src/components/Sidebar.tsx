import { Home, FileText, Workflow, Wrench, Plus, Trash2 } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";
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
          <Button
            key={view}
            variant="ghost"
            size="sm"
            onClick={() => onViewChange(view)}
            className={`flex w-full justify-start gap-2.5 ${
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

      {/* Flow list (shown when Flows view is active) */}
      {activeView === "flows" && (
        <div className="mt-2 flex-1 overflow-auto border-t border-border pt-2">
          <div className="flex items-center justify-between px-3 pb-1">
            <span className="text-xs font-medium text-muted-foreground">Flows</span>
            {onAddFlow && (
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={onAddFlow}
                className="h-5 w-5"
                aria-label="New flow"
              >
                <Plus size={12} />
              </Button>
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
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => onDeleteFlow(name)}
                    className="h-4 w-4 opacity-0 hover:text-destructive group-hover:opacity-100"
                    aria-label={`Delete flow ${name}`}
                  >
                    <Trash2 size={10} />
                  </Button>
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
