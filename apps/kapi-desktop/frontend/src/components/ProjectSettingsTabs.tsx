import type { ReactNode } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { SubNavTabs } from "./SubNavTabs";

/**
 * The tabs of Project Settings besides the settings themselves: the project's
 * content collections and where they stand, the assistants connected to it,
 * and sharing its context. Sessions is reached from the Assistants page, so it
 * has no tab of its own and keeps Assistants marked.
 */
export const SETTINGS_SECTIONS = ["content", "assistants", "sessions", "share"] as const;
export type SettingsSection = (typeof SETTINGS_SECTIONS)[number];

type TabID = "settings" | Exclude<SettingsSection, "sessions">;

const TABS: Array<{ id: TabID; label: () => string }> = [
  { id: "settings", label: () => t("Settings") },
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
  const marked: TabID =
    section === null ? "settings" : section === "sessions" ? "assistants" : section;
  const page = section === null ? pages.settings : (pages[section] ?? pages.assistants);
  return (
    <div data-slot="project-settings-tabs" className="flex h-full min-h-0 flex-col">
      <SubNavTabs
        label={t("Project Settings sections")}
        tabs={TABS.map((tab) => ({ id: tab.id, label: tab.label() }))}
        active={marked}
        onSelect={(id) =>
          onNavigate(id === "settings" ? "project-settings" : `project-settings:${id}`)
        }
      />
      <div className="min-h-0 flex-1 overflow-auto">{page}</div>
    </div>
  );
}
