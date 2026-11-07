import type { ReactNode } from "react";

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
      <div className="sticky top-0 z-10 flex items-center gap-1 border-b border-border bg-background/95 px-4 backdrop-blur">
        <TabButton active={tab === "tools"} onClick={() => onTabChange("tools")}>
          Tools
        </TabButton>
        <TabButton active={tab === "flows"} onClick={() => onTabChange("flows")}>
          Flows
        </TabButton>
        <TabButton active={tab === "formats"} onClick={() => onTabChange("formats")}>
          Formats
        </TabButton>
      </div>
      {tab === "flows" ? flows : tab === "formats" ? formats : tools}
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
