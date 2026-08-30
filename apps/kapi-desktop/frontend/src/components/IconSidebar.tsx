import {
  Home,
  Compass,
  FolderKanban,
  BookOpen,
  ClipboardCheck,
  Database,
  Wrench,
  FileText,
  Settings,
  SlidersHorizontal,
  ShieldCheck,
} from "lucide-react";
import { SimpleTooltip } from "@neokapi/ui-primitives";

const SW = 1.5;

type SidebarItem =
  | {
      type: "item";
      view: string;
      icon: React.ReactNode;
      label: string;
      alwaysEnabled?: boolean;
      /** When true, this item appears only once the project has target
       *  languages (e.g. Content Memory). Hidden, not disabled, when it
       *  doesn't — a project simply shows the surfaces its languages call for. */
      localeGated?: boolean;
      /** Extra routes that keep this item highlighted — e.g. the Toolbox stays
       *  active on its "flows" sub-tab. */
      activeViews?: string[];
    }
  | { type: "separator" };

interface IconSidebarProps {
  mode: "adhoc" | "projects";
  active: string;
  onChange: (view: string) => void;
  projectDisabled?: boolean;
  /** Whether the open project has target languages. Locale-gated items appear
   *  only when it does — the source-first, "languages stay quiet" model. */
  hasTargetLanguages?: boolean;
}

const adhocItems: SidebarItem[] = [
  {
    type: "item",
    view: "home",
    icon: <Home size={20} strokeWidth={SW} />,
    label: "Home",
  },
  { type: "separator" },
  {
    type: "item",
    view: "tools",
    icon: <Wrench size={20} strokeWidth={SW} />,
    label: "Toolbox",
    activeViews: ["flows"],
  },
  {
    type: "item",
    view: "termbases",
    icon: <BookOpen size={20} strokeWidth={SW} />,
    label: "Terms",
  },
  {
    type: "item",
    view: "memories",
    icon: <Database size={20} strokeWidth={SW} />,
    label: "Content Memory",
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
    view: "context",
    icon: <Compass size={20} strokeWidth={SW} />,
    label: "Context",
    // Terms and Content Memory are sections of the hub; their view ids stay
    // routable so a persisted view still opens where it used to.
    activeViews: ["termbases", "memories"],
  },
  {
    type: "item",
    view: "checks",
    icon: <ShieldCheck size={20} strokeWidth={SW} />,
    label: "Checks",
  },
  {
    type: "item",
    view: "review",
    icon: <ClipboardCheck size={20} strokeWidth={SW} />,
    label: "Review",
    localeGated: true,
  },
  {
    type: "item",
    view: "tools",
    icon: <Wrench size={20} strokeWidth={SW} />,
    label: "Toolbox",
    activeViews: ["flows"],
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
  hasTargetLanguages,
}: IconSidebarProps) {
  const items = mode === "adhoc" ? adhocItems : projectItems;

  return (
    <aside className="flex h-full w-12 flex-col items-center py-2">
      <nav className="flex flex-1 flex-col items-center gap-1">
        {items.map((item, i) => {
          if (item.type === "separator") {
            return <div key={`sep-${i}`} className="my-1 h-px w-6 bg-border" />;
          }
          // Locale-gated items (Content Memory) appear only once the
          // project has target languages — present, never an announced unlock.
          if (item.localeGated && !hasTargetLanguages) {
            return null;
          }
          const disabled = mode === "projects" && projectDisabled && !item.alwaysEnabled;
          const isActive = active === item.view || (item.activeViews?.includes(active) ?? false);
          const title = disabled ? `${item.label} (open a project first)` : item.label;
          return (
            <SimpleTooltip key={item.view} content={title} side="right">
              <span className="inline-flex">
                <button
                  onClick={() => !disabled && onChange(item.view)}
                  disabled={disabled}
                  className={`rounded-lg p-2 transition-colors ${
                    isActive
                      ? "bg-primary text-primary-foreground"
                      : disabled
                        ? "text-muted-foreground/30 cursor-not-allowed"
                        : "text-muted-foreground hover:bg-accent hover:text-foreground"
                  }`}
                  aria-label={item.label}
                >
                  {item.icon}
                </button>
              </span>
            </SimpleTooltip>
          );
        })}
      </nav>

      {/* Global Settings — pinned at bottom */}
      <div className="flex flex-col items-center">
        <SimpleTooltip content="App Settings" side="right">
          <button
            onClick={() => onChange("app-settings")}
            className={`rounded-lg p-2 transition-colors ${
              active === "app-settings"
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:bg-accent hover:text-foreground"
            }`}
            aria-label="App Settings"
          >
            <Settings size={20} strokeWidth={SW} />
          </button>
        </SimpleTooltip>
      </div>
    </aside>
  );
}
