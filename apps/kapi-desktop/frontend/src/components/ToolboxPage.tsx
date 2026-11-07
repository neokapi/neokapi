import type { ReactNode } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { SubNavTabs } from "./SubNavTabs";

export interface ToolboxPageProps {
  /** Active sub-view, driven by the route ("tools" | "flows" | "formats"). */
  tab: "tools" | "flows" | "formats";
  /** Switch sub-view — navigates the route so the tab is deep-linkable. */
  onTabChange: (view: string) => void;
  /** The single-tools runner. */
  tools: ReactNode;
  /** The flows library / editor. */
  flows: ReactNode;
  /** The formats kapi reads and writes, with their settings. */
  formats: ReactNode;
}

/**
 * The Toolbox hosts single Tools, saved Flows and the Formats as three
 * route-driven tabs. Flows are authored and managed here: the library, where a
 * flow is created, edited, deleted and run. The Toolbox is the only place
 * these three pages open from.
 */
export function ToolboxPage({ tab, onTabChange, tools, flows, formats }: ToolboxPageProps) {
  return (
    <div>
      <SubNavTabs
        label={t("Toolbox sections")}
        tabs={[
          { id: "tools", label: t("Tools") },
          { id: "flows", label: t("Flows") },
          { id: "formats", label: t("Formats") },
        ]}
        active={tab}
        onSelect={onTabChange}
      />
      {tab === "flows" ? flows : tab === "formats" ? formats : tools}
    </div>
  );
}
