import {
  Home,
  FolderKanban,
  BookOpen,
  Database,
  Workflow,
  Wrench,
  FileText,
  Settings,
  SlidersHorizontal,
  ShieldCheck,
} from "lucide-react";

const SW = 1.5;

type SidebarItem =
  | {
      type: "item";
      view: string;
      icon: React.ReactNode;
      label: string;
      alwaysEnabled?: boolean;
      /** When true, this item is disabled when project plugins are unresolved. */
      pluginGated?: boolean;
    }
  | { type: "separator" };

interface IconSidebarProps {
  mode: "adhoc" | "projects";
  active: string;
  onChange: (view: string) => void;
  projectDisabled?: boolean;
  /** When true, plugin-gated items (Content, Flows) are disabled. */
  pluginsUnresolved?: boolean;
}

const adhocItems: SidebarItem[] = [
  { type: "item", view: "home", icon: <Home size={20} strokeWidth={SW} />, label: "Home" },
  { type: "separator" },
  { type: "item", view: "flows", icon: <Workflow size={20} strokeWidth={SW} />, label: "Flows" },
  { type: "item", view: "tools", icon: <Wrench size={20} strokeWidth={SW} />, label: "Tools" },
  {
    type: "item",
    view: "termbases",
    icon: <BookOpen size={20} strokeWidth={SW} />,
    label: "Termbases",
  },
  {
    type: "item",
    view: "memories",
    icon: <Database size={20} strokeWidth={SW} />,
    label: "Translation Memories",
  },
  {
    type: "item",
    view: "formats",
    icon: <FileText size={20} strokeWidth={SW} />,
    label: "Formats",
  },
];

const projectItems: SidebarItem[] = [
  {
    type: "item",
    view: "home",
    icon: <Home size={20} strokeWidth={SW} />,
    label: "Home",
    alwaysEnabled: true,
  },
  { type: "separator" },
  {
    type: "item",
    view: "project-home",
    icon: <FolderKanban size={20} strokeWidth={SW} />,
    label: "Project",
  },
  {
    type: "item",
    view: "content",
    icon: <FileText size={20} strokeWidth={SW} />,
    label: "Content",
    pluginGated: true,
  },
  {
    type: "item",
    view: "flows",
    icon: <Workflow size={20} strokeWidth={SW} />,
    label: "Flows",
    pluginGated: true,
  },
  { type: "item", view: "tools", icon: <Wrench size={20} strokeWidth={SW} />, label: "Tools" },
  {
    type: "item",
    view: "checks",
    icon: <ShieldCheck size={20} strokeWidth={SW} />,
    label: "Checks",
    pluginGated: true,
  },
  {
    type: "item",
    view: "termbases",
    icon: <BookOpen size={20} strokeWidth={SW} />,
    label: "Termbases",
  },
  {
    type: "item",
    view: "memories",
    icon: <Database size={20} strokeWidth={SW} />,
    label: "Translation Memories",
  },
  {
    type: "item",
    view: "project-settings",
    icon: <SlidersHorizontal size={20} strokeWidth={SW} />,
    label: "Project Settings",
  },
];

export function IconSidebar({
  mode,
  active,
  onChange,
  projectDisabled,
  pluginsUnresolved,
}: IconSidebarProps) {
  const items = mode === "adhoc" ? adhocItems : projectItems;

  return (
    <aside className="flex h-full w-12 flex-col items-center py-2">
      <nav className="flex flex-1 flex-col items-center gap-1">
        {items.map((item, i) => {
          if (item.type === "separator") {
            return <div key={`sep-${i}`} className="my-1 h-px w-6 bg-border" />;
          }
          const noProject = mode === "projects" && projectDisabled && !item.alwaysEnabled;
          const pluginBlocked = !!(pluginsUnresolved && item.pluginGated);
          const disabled = noProject || pluginBlocked;
          const title = noProject
            ? `${item.label} (open a project first)`
            : pluginBlocked
              ? `${item.label} (resolve plugin requirements in Settings)`
              : item.label;
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
              title={title}
            >
              {item.icon}
            </button>
          );
        })}
      </nav>

      {/* Global Settings — pinned at bottom */}
      <div className="flex flex-col items-center">
        <button
          onClick={() => onChange("app-settings")}
          className={`rounded-lg p-2 transition-colors ${
            active === "app-settings"
              ? "bg-primary text-primary-foreground"
              : "text-muted-foreground hover:bg-accent hover:text-foreground"
          }`}
          aria-label="App Settings"
          title="App Settings"
        >
          <Settings size={20} strokeWidth={SW} />
        </button>
      </div>
    </aside>
  );
}
