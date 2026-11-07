import {
  Home,
  Compass,
  ClipboardCheck,
  Wrench,
  LayoutDashboard,
  Settings,
  SlidersHorizontal,
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
      /** Extra routes that keep this item highlighted, such as the Toolbox on
       *  its "flows" tab. */
      activeViews?: string[];
    }
  | { type: "separator" };

interface IconSidebarProps {
  active: string;
  onChange: (view: string) => void;
  projectDisabled?: boolean;
}

// One entry per page, and one route to each page. The view ids are persisted
// in the session (backend/settings.go), so the Overview keeps the "map" id and
// the ids of the pages it absorbed stay highlighted under it.
const projectItems: SidebarItem[] = [
  {
    type: "item",
    view: "home",
    icon: <Home size={20} strokeWidth={SW} />,
    label: "Home",
    alwaysEnabled: true,
    // The workspace's history opens from the home and belongs to it.
    activeViews: ["history"],
  },
  { type: "separator" },
  {
    type: "item",
    view: "map",
    icon: <LayoutDashboard size={20} strokeWidth={SW} />,
    label: "Overview",
    // The findings open from the Overview's count and a file opens in the
    // Reader from its Files rows, so both stay lit under it. "project-home",
    // "content" and "languages" are the persisted ids of pages it replaced.
    activeViews: ["checks", "reader", "setup", "project-home", "content", "languages"],
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
    view: "review",
    icon: <ClipboardCheck size={20} strokeWidth={SW} />,
    label: "Review",
  },
  {
    type: "item",
    view: "tools",
    icon: <Wrench size={20} strokeWidth={SW} />,
    label: "Toolbox",
    activeViews: ["flows", "formats"],
  },
  {
    type: "item",
    view: "project-settings",
    icon: <SlidersHorizontal size={20} strokeWidth={SW} />,
    label: "Project Settings",
  },
];

export function IconSidebar({ active, onChange, projectDisabled }: IconSidebarProps) {
  return (
    <aside className="flex h-full w-12 flex-col items-center py-2">
      <nav className="flex flex-1 flex-col items-center gap-1">
        {projectItems.map((item, i) => {
          if (item.type === "separator") {
            return <div key={`sep-${i}`} className="my-1 h-px w-6 bg-border" />;
          }
          const disabled = projectDisabled && !item.alwaysEnabled;
          const isActive =
            active === item.view ||
            active.startsWith(`${item.view}:`) ||
            (item.activeViews?.some((v) => active === v || active.startsWith(`${v}:`)) ?? false);
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
