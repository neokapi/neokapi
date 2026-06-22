import type { ReactNode } from "react";

export interface ToolboxPageProps {
  /** Active sub-view, driven by the route ("tools" | "flows"). */
  tab: "tools" | "flows";
  /** Switch sub-view — navigates the route so the tab is deep-linkable. */
  onTabChange: (view: string) => void;
  /** The single-tools runner. */
  tools: ReactNode;
  /** The flows library / editor. */
  flows: ReactNode;
}

/**
 * The Toolbox hosts single Tools and saved Flows as two route-driven tabs.
 *
 * Flows are no longer a sidebar pillar — they're authored and managed here (the
 * library: create / edit / delete, opening the flow editor). Running a flow
 * against a project's content stays on the project Home, where the content
 * collections live; this is just the workbench where flows are built.
 */
export function ToolboxPage({ tab, onTabChange, tools, flows }: ToolboxPageProps) {
  return (
    <div>
      <div className="sticky top-0 z-10 flex items-center gap-1 border-b border-border bg-background/95 px-4 backdrop-blur">
        <TabButton active={tab === "tools"} onClick={() => onTabChange("tools")}>
          Tools
        </TabButton>
        <TabButton active={tab === "flows"} onClick={() => onTabChange("flows")}>
          Flows
        </TabButton>
      </div>
      {tab === "flows" ? flows : tools}
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
