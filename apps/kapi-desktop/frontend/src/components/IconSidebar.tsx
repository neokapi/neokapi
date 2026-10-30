import {
  Home,
  Compass,
  FolderKanban,
  ClipboardCheck,
  Languages,
  Plus,
  Wrench,
  Map as MapIcon,
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
      /** When true, this item appears only once the project declares a target
       *  language (Review). Hidden, not disabled, when it does not: a project
       *  shows the surfaces its languages call for. */
      localeGated?: boolean;
      /** The Languages door: "Languages +" until the project declares a
       *  target language, "Languages" from then on. */
      languages?: boolean;
      /** Extra routes that keep this item highlighted — e.g. the Toolbox stays
       *  active on its "flows" sub-tab. */
      activeViews?: string[];
    }
  | { type: "separator" };

interface IconSidebarProps {
  active: string;
  onChange: (view: string) => void;
  projectDisabled?: boolean;
  /** Whether the open project's recipe declares a target language
   *  (`declaresTargetLanguages`). Review appears only when it does, and the
   *  Languages entry reads "Languages +" until then. */
  hasTargetLanguages?: boolean;
}

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
    icon: <MapIcon size={20} strokeWidth={SW} />,
    label: "Map",
    // The findings open from the Map's count and a file opens in the Reader
    // from its Files rows, so the Checks page and the Reader stay lit under it.
    activeViews: ["checks", "reader"],
  },
  {
    type: "item",
    view: "context",
    icon: <Compass size={20} strokeWidth={SW} />,
    label: "Context",
    // Terms and Content Memory are sections of the hub; their view ids stay
    // routable so a persisted view still opens where it used to. A view of
    // the form "context:<section>" opens the hub on one section.
    activeViews: ["termbases", "memories"],
  },
  {
    type: "item",
    view: "project-home",
    icon: <FolderKanban size={20} strokeWidth={SW} />,
    label: "Project",
  },
  {
    type: "item",
    view: "languages",
    icon: <Languages size={20} strokeWidth={SW} />,
    label: "Languages",
    languages: true,
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
  active,
  onChange,
  projectDisabled,
  hasTargetLanguages,
}: IconSidebarProps) {
  const items = projectItems;

  return (
    <aside className="flex h-full w-12 flex-col items-center py-2">
      <nav className="flex flex-1 flex-col items-center gap-1">
        {items.map((item, i) => {
          if (item.type === "separator") {
            return <div key={`sep-${i}`} className="my-1 h-px w-6 bg-border" />;
          }
          // Review appears only once the project declares a target language.
          if (item.localeGated && !hasTargetLanguages) {
            return null;
          }
          // Without a target language the Languages door carries a plus, and
          // a persisted Review view, which opens the door too, keeps it lit.
          const door = item.languages && !hasTargetLanguages;
          const label = door ? "Languages +" : item.label;
          const activeViews = door ? [...(item.activeViews ?? []), "review"] : item.activeViews;
          const disabled = projectDisabled && !item.alwaysEnabled;
          const isActive =
            active === item.view ||
            active.startsWith(`${item.view}:`) ||
            (activeViews?.some((v) => active === v || active.startsWith(`${v}:`)) ?? false);
          const title = disabled ? `${label} (open a project first)` : label;
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
                  aria-label={label}
                >
                  {door ? (
                    <span className="relative inline-flex" data-slot="languages-door">
                      {item.icon}
                      <Plus
                        size={10}
                        strokeWidth={2.5}
                        aria-hidden
                        className="absolute -right-1.5 -bottom-1"
                      />
                    </span>
                  ) : (
                    item.icon
                  )}
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
