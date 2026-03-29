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

const SW = 1.5; // stroke width for all icons

const topItems: { section: AppSection; icon: React.ReactNode; label: string }[] = [
  { section: "home", icon: <Home size={20} strokeWidth={SW} />, label: "Home" },
  { section: "projects", icon: <FolderKanban size={20} strokeWidth={SW} />, label: "Projects" },
  { section: "termbases", icon: <BookOpen size={20} strokeWidth={SW} />, label: "Termbases" },
  { section: "memories", icon: <Database size={20} strokeWidth={SW} />, label: "Translation Memories" },
  { section: "flows", icon: <Workflow size={20} strokeWidth={SW} />, label: "Flows" },
  { section: "tools", icon: <Wrench size={20} strokeWidth={SW} />, label: "Tools" },
  { section: "formats", icon: <FileText size={20} strokeWidth={SW} />, label: "Formats" },
];

export function IconRail({ active, onChange }: IconRailProps) {
  return (
    <aside className="flex w-12 flex-col items-center py-2">
      <nav className="flex flex-1 flex-col items-center gap-1">
        {topItems.map(({ section, icon, label }) => (
          <button
            key={section}
            onClick={() => onChange(section)}
            className={`relative rounded-lg p-2 transition-colors ${
              active === section
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:bg-accent hover:text-foreground"
            }`}
            aria-label={label}
            title={label}
          >
            {icon}
          </button>
        ))}
      </nav>

      <button
        onClick={() => onChange("settings")}
        className={`rounded-lg p-2 transition-colors ${
          active === "settings"
            ? "bg-primary text-primary-foreground"
            : "text-muted-foreground hover:bg-accent hover:text-foreground"
        }`}
        aria-label="Settings"
        title="Settings"
      >
        <Settings size={20} strokeWidth={SW} />
      </button>
    </aside>
  );
}
