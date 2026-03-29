import {
  Home,
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
  /** When true, project items (except home) are grayed out and not clickable. */
  projectDisabled?: boolean;
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
  { view: "home", icon: <Home size={20} strokeWidth={SW} />, label: "Home", alwaysEnabled: true },
  { view: "content", icon: <FileText size={20} strokeWidth={SW} />, label: "Content", alwaysEnabled: false },
  { view: "flows", icon: <Workflow size={20} strokeWidth={SW} />, label: "Flows", alwaysEnabled: false },
  { view: "tools", icon: <Wrench size={20} strokeWidth={SW} />, label: "Tools", alwaysEnabled: false },
];

export function IconSidebar({ mode, active, onChange, projectDisabled }: IconSidebarProps) {
  const items = mode === "adhoc" ? adhocItems : projectItems;

  return (
    <aside className="flex w-12 flex-col items-center py-2">
      <nav className="flex flex-1 flex-col items-center gap-1">
        {items.map((item) => {
          const disabled = mode === "projects" && projectDisabled && !("alwaysEnabled" in item && item.alwaysEnabled);
          return (
            <button
              key={item.view}
              onClick={() => !disabled && onChange(item.view)}
              disabled={disabled}
              className={`rounded-lg p-2 transition-colors ${
                active === item.view
                  ? "bg-primary text-primary-foreground"
                  : disabled
                    ? "text-muted-foreground/30 cursor-not-allowed"
                    : "text-muted-foreground hover:bg-accent hover:text-foreground"
              }`}
              aria-label={item.label}
              title={disabled ? `${item.label} (open a project first)` : item.label}
            >
              {item.icon}
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
