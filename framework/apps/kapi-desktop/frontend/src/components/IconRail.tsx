import {
  Home,
  ArrowLeft,
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
  projectActive?: boolean;
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

// When a project is active, only show Home and Projects.
const projectOnlyItems = new Set<AppSection>(["home", "projects"]);

export function IconRail({ active, onChange, projectActive }: IconRailProps) {
  const visibleItems = projectActive
    ? topItems.filter((i) => projectOnlyItems.has(i.section))
    : topItems;

  return (
    <aside className="flex w-12 flex-col items-center py-2">
      <nav className="flex flex-1 flex-col items-center gap-1">
        {visibleItems.map(({ section, icon, label }) => {
          const isBack = projectActive && section === "home";
          return (
            <button
              key={section}
              onClick={() => onChange(section)}
              className={`relative rounded-lg p-2 transition-colors ${
                active === section
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-accent hover:text-foreground"
              }`}
              aria-label={isBack ? "Back" : label}
              title={isBack ? "Back" : label}
            >
              {isBack ? <ArrowLeft size={20} strokeWidth={SW} /> : icon}
            </button>
          );
        })}
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
