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

const SW = 1.5;

interface IconSidebarProps {
  mode: "adhoc" | "projects";
  active: string;
  onChange: (view: string) => void;
}

const adhocItems = [
  { view: "home", icon: <Home size={20} strokeWidth={SW} />, label: "Home" },
  { view: "flows", icon: <Workflow size={20} strokeWidth={SW} />, label: "Flows" },
  { view: "tools", icon: <Wrench size={20} strokeWidth={SW} />, label: "Tools" },
  { view: "termbases", icon: <BookOpen size={20} strokeWidth={SW} />, label: "Termbases" },
  { view: "memories", icon: <Database size={20} strokeWidth={SW} />, label: "Translation Memories" },
  { view: "formats", icon: <FileText size={20} strokeWidth={SW} />, label: "Formats" },
];

const projectItems = [
  { view: "home", icon: <FolderKanban size={20} strokeWidth={SW} />, label: "Project Home" },
  { view: "content", icon: <FileText size={20} strokeWidth={SW} />, label: "Content" },
  { view: "flows", icon: <Workflow size={20} strokeWidth={SW} />, label: "Flows" },
  { view: "tools", icon: <Wrench size={20} strokeWidth={SW} />, label: "Tools" },
];

export function IconSidebar({ mode, active, onChange }: IconSidebarProps) {
  const items = mode === "adhoc" ? adhocItems : projectItems;

  return (
    <aside className="flex w-12 flex-col items-center py-2">
      <nav className="flex flex-1 flex-col items-center gap-1">
        {items.map(({ view, icon, label }) => (
          <button
            key={view}
            onClick={() => onChange(view)}
            className={`rounded-lg p-2 transition-colors ${
              active === view
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
