import {
  FolderOpen,
  Workflow,
  Wrench,
  Settings,
} from "lucide-react";
import type { View } from "../types/api";

interface SidebarProps {
  activeView: View;
  onViewChange: (view: View) => void;
}

const projectNavItems: { view: View; label: string; icon: React.ReactNode }[] = [
  { view: "project", label: "Project", icon: <FolderOpen size={18} /> },
  { view: "flows", label: "Flows", icon: <Workflow size={18} /> },
  { view: "tools", label: "Tools", icon: <Wrench size={18} /> },
];

export function Sidebar({
  activeView,
  onViewChange,
}: SidebarProps) {
  return (
    <aside className="flex w-52 flex-col border-r border-border bg-sidebar pt-2">
      {/* Project navigation */}
      <nav className="flex-1 space-y-0.5 px-2">
        {projectNavItems.map(({ view, label, icon }) => (
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
          <Settings size={18} />
          Settings
        </button>
      </div>
    </aside>
  );
}
