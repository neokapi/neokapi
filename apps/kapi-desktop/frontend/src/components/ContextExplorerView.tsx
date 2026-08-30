import { useMemo } from "react";
import { PageHeader } from "@neokapi/ui-primitives";
import { ContextExplorer, ScopeProvider } from "@neokapi/context-explorer";
import type { ContextDataSource, Dimension } from "@neokapi/context-explorer";
import { createLocalContextSource } from "../lib/localContextSource";

/** Workspace, project and stream are the tab's; the reader moves the rest. */
const PINNED: Dimension[] = ["workspace", "project", "stream"];

export interface ContextExplorerViewProps {
  tabID: string;
  /** The open project's name — the ladder's project rung. */
  projectName: string;
  /** Injected in tests and stories; production reads the Wails backend. */
  source?: ContextDataSource;
}

/**
 * The context explorer for the open project.
 *
 * The same component set Bowrain renders across a workspace, with the
 * dimensions pinned to this tab: a desktop project is one project on one
 * stream, so the ladder starts at its collections and the panes state the
 * project reach of what they can see.
 */
export function ContextExplorerView({ tabID, projectName, source }: ContextExplorerViewProps) {
  const client = useMemo(() => source ?? createLocalContextSource(tabID), [source, tabID]);
  const scope = useMemo(
    () => ({ project: projectName || "project", stream: "main" }),
    [projectName],
  );

  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto p-6">
      <PageHeader
        title="Explorer"
        subtitle="What governs a point in this project, what lives there, and how it relates."
      />
      <ScopeProvider source={client} pinned={PINNED} scope={scope}>
        <ContextExplorer className="mt-4" />
      </ScopeProvider>
    </div>
  );
}
