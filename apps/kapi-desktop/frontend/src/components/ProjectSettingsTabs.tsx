import type { ReactNode } from "react";
import { t } from "@neokapi/i18n-react/runtime";

/**
 * The sections of Project Settings → Tools. Each renders the app-wide page of
 * the same name: the tools and flows, terms, content memories and formats
 * that stand outside any one project.
 */
export const SETTINGS_TOOL_SECTIONS = ["tools", "flows", "terms", "memories", "formats"] as const;
export type SettingsToolSection = (typeof SETTINGS_TOOL_SECTIONS)[number];

/**
 * Which part of Project Settings a view names. The view is
 * "project-settings" for the settings themselves, and
 * "project-settings:<section>" for one section of Tools.
 */
export function settingsSectionOf(view: string): SettingsToolSection | null {
  const section = view.split(":")[1];
  return (SETTINGS_TOOL_SECTIONS as readonly string[]).includes(section)
    ? (section as SettingsToolSection)
    : null;
}

export interface ProjectSettingsTabsProps {
  /** The section of Tools on view, or null for the project's settings. */
  section: SettingsToolSection | null;
  /** Navigate to a view; the tabs are routes, so each is deep-linkable. */
  onNavigate: (view: string) => void;
  /** The project's settings. */
  settings: ReactNode;
  /** The page for each section of Tools. */
  tools: Record<SettingsToolSection, ReactNode>;
}

const SECTION_LABELS: Record<SettingsToolSection, () => string> = {
  tools: () => t("Tools"),
  flows: () => t("Flows"),
  terms: () => t("Terms"),
  memories: () => t("Content memory"),
  formats: () => t("Formats"),
};

/**
 * Project Settings as two tabs: the project's own settings, and Tools, which
 * holds the app-wide pages a person reaches without a project in mind.
 */
export function ProjectSettingsTabs({
  section,
  onNavigate,
  settings,
  tools,
}: ProjectSettingsTabsProps) {
  return (
    <div data-slot="project-settings-tabs" className="flex h-full min-h-0 flex-col">
      <div
        role="tablist"
        className="sticky top-0 z-10 flex shrink-0 items-center gap-1 border-b border-border bg-background/95 px-4 backdrop-blur"
      >
        <TabButton active={section === null} onClick={() => onNavigate("project-settings")}>
          {t("Settings")}
        </TabButton>
        <TabButton active={section !== null} onClick={() => onNavigate("project-settings:tools")}>
          {t("Tools")}
        </TabButton>
      </div>
      {section === null ? (
        <div className="min-h-0 flex-1 overflow-auto">{settings}</div>
      ) : (
        <>
          <div
            data-slot="settings-tool-sections"
            role="tablist"
            className="flex shrink-0 items-center gap-1 border-b border-border/60 px-4"
          >
            {SETTINGS_TOOL_SECTIONS.map((s) => (
              <TabButton
                key={s}
                small
                active={s === section}
                onClick={() => onNavigate(`project-settings:${s}`)}
              >
                {SECTION_LABELS[s]()}
              </TabButton>
            ))}
          </div>
          <div className="min-h-0 flex-1 overflow-auto">{tools[section]}</div>
        </>
      )}
    </div>
  );
}

function TabButton({
  active,
  small,
  onClick,
  children,
}: {
  active: boolean;
  small?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={`-mb-px border-b-2 px-3 transition-colors ${small ? "py-2 text-xs" : "py-2.5 text-sm"} ${
        active
          ? "border-primary font-medium text-foreground"
          : "border-transparent text-muted-foreground hover:text-foreground"
      }`}
    >
      {children}
    </button>
  );
}
