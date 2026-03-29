import {
  Home,
  FolderKanban,
  BookOpen,
  Database,
  Workflow,
  Wrench,
  FileText,
  Settings,
} from "lucide-react";
import type { AppSection } from "../types/api";

interface IconRailProps {
  active: AppSection;
  onChange: (section: AppSection) => void;
}

const topItems: { section: AppSection; icon: React.ReactNode; label: string }[] = [
  { section: "home", icon: <Home size={20} />, label: "Home" },
  { section: "projects", icon: <FolderKanban size={20} />, label: "Projects" },
  { section: "termbases", icon: <BookOpen size={20} />, label: "Termbases" },
  { section: "memories", icon: <Database size={20} />, label: "Translation Memories" },
  { section: "flows", icon: <Workflow size={20} />, label: "Flows" },
  { section: "tools", icon: <Wrench size={20} />, label: "Tools" },
  { section: "formats", icon: <FileText size={20} />, label: "Formats" },
];

export function IconRail({ active, onChange }: IconRailProps) {
  return (
    <aside className="flex w-12 flex-col items-center border-r border-border bg-sidebar py-2">
      <nav className="flex flex-1 flex-col items-center gap-1">
        {topItems.map(({ section, icon, label }) => (
          <button
            key={section}
            onClick={() => onChange(section)}
            className={`relative rounded-lg p-2 transition-colors ${
              active === section
                ? "bg-accent text-foreground"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
            }`}
            aria-label={label}
            title={label}
          >
            {icon}
          </button>
        ))}
      </nav>

      {/* Settings pinned to bottom */}
      <button
        onClick={() => onChange("settings")}
        className={`rounded-lg p-2 transition-colors ${
          active === "settings"
            ? "bg-accent text-foreground"
            : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
        }`}
        aria-label="Settings"
        title="Settings"
      >
        <Settings size={20} />
      </button>
    </aside>
  );
}
