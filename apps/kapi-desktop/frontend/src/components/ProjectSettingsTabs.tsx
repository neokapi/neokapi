import type { ReactNode } from "react";
import { t } from "@neokapi/i18n-react/runtime";

/**
 * The tabs of Project Settings besides the settings themselves: the project's
 * content collections and where they stand, the assistants connected to it,
 * and sharing its context. Sessions is reached from the Assistants page, so it
 * has no tab of its own and keeps Assistants marked.
 */
export const SETTINGS_SECTIONS = ["content", "assistants", "sessions", "share"] as const;
export type SettingsSection = (typeof SETTINGS_SECTIONS)[number];

const TABS: Array<{ id: SettingsSection | null; label: () => string }> = [
  { id: null, label: () => t("Settings") },
  { id: "content", label: () => t("Content") },
  { id: "assistants", label: () => t("Assistants") },
  { id: "share", label: () => t("Share") },
];

/**
 * Which part of Project Settings a view names. The view is
 * "project-settings" for the settings themselves, and
 * "project-settings:<section>" for one of the other tabs.
 */
export function settingsSectionOf(view: string): SettingsSection | null {
  const section = view.split(":")[1];
  return (SETTINGS_SECTIONS as readonly string[]).includes(section)
    ? (section as SettingsSection)
    : null;
}

export interface ProjectSettingsTabsProps {
  /** The tab on view, or null for the project's settings. */
  section: SettingsSection | null;
  /** Navigate to a view; the tabs are routes, so each is deep-linkable. */
  onNavigate: (view: string) => void;
  /** The page for each tab. */
  pages: { settings: ReactNode } & Record<Exclude<SettingsSection, "sessions">, ReactNode> & {
      sessions?: ReactNode;
    };
}

/** Project Settings as tabs: one page per tab, each a route. */
export function ProjectSettingsTabs({ section, onNavigate, pages }: ProjectSettingsTabsProps) {
  const marked = section === "sessions" ? "assistants" : section;
  const page = section === null ? pages.settings : (pages[section] ?? pages.assistants);
  return (
    <div data-slot="project-settings-tabs" className="flex h-full min-h-0 flex-col">
      <div
        role="tablist"
        className="sticky top-0 z-10 flex shrink-0 items-center gap-1 border-b border-border bg-background/95 px-4 backdrop-blur"
      >
        {TABS.map((tab) => (
          <TabButton
            key={tab.id ?? "settings"}
            active={marked === tab.id}
            onClick={() =>
              onNavigate(tab.id === null ? "project-settings" : `project-settings:${tab.id}`)
            }
          >
            {tab.label()}
          </TabButton>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-auto">{page}</div>
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={`-mb-px border-b-2 px-3 py-2.5 text-sm transition-colors ${
        active
          ? "border-primary font-medium text-foreground"
          : "border-transparent text-muted-foreground hover:text-foreground"
      }`}
    >
      {children}
    </button>
  );
}
