import {
  FolderOpen,
  Workflow,
  Wrench,
} from "lucide-react";
import type { View } from "../types/api";

interface SidebarProps {
  activeView: View;
  onViewChange: (view: View) => void;
}

const navItems: { view: View; label: string; icon: React.ReactNode }[] = [
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
      <nav className="flex-1 space-y-0.5 px-2">
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
    </aside>
  );
}
