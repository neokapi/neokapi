import { useState, useEffect } from "react";
import {
  FolderOpen,
  Workflow,
  Wrench,
  Puzzle,
  KeyRound,
  Settings,
  X,
} from "lucide-react";
import type { View } from "../types/api";
import { api } from "../hooks/useApi";

interface SidebarProps {
  activeView: View;
  onViewChange: (view: View) => void;
  projectName?: string;
  onCloseProject: () => void;
}

const navItems: { view: View; label: string; icon: React.ReactNode }[] = [
  { view: "project", label: "Project", icon: <FolderOpen size={18} /> },
  { view: "flows", label: "Flows", icon: <Workflow size={18} /> },
  { view: "tools", label: "Tools", icon: <Wrench size={18} /> },
  { view: "plugins", label: "Plugins", icon: <Puzzle size={18} /> },
  { view: "credentials", label: "Credentials", icon: <KeyRound size={18} /> },
  { view: "settings", label: "Settings", icon: <Settings size={18} /> },
];

export function Sidebar({
  activeView,
  onViewChange,
  projectName,
  onCloseProject,
}: SidebarProps) {
  const [version, setVersion] = useState("v0.1.0");

  useEffect(() => {
    api.getVersion().then((v) => {
      if (v) setVersion(v);
    });
  }, []);

  return (
    <aside className="flex w-52 flex-col border-r border-border bg-sidebar">
      {/* Drag region for macOS title bar */}
      <div className="h-12 shrink-0" style={{ WebkitAppRegion: "drag" } as React.CSSProperties} />

      {/* Project name */}
      {projectName && (
        <div className="flex items-center gap-2 px-3 pb-2">
          <span className="truncate text-sm font-medium">{projectName}</span>
          <button
            onClick={onCloseProject}
            className="ml-auto rounded p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
            title="Close project"
          >
            <X size={14} />
          </button>
        </div>
      )}

      {/* Navigation */}
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

      {/* Version */}
      <div className="border-t border-border p-3">
        <span className="text-xs text-muted-foreground">Kapi Desktop {version}</span>
      </div>
    </aside>
  );
}
