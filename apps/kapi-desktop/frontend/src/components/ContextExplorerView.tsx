import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Badge, PageHeader } from "@neokapi/ui-primitives";
import { ContextExplorer, ScopeProvider } from "@neokapi/context-explorer";
import type { ContextDataSource, Dimension } from "@neokapi/context-explorer";
import { t } from "@neokapi/i18n-react/runtime";
import { createLocalContextSource } from "../lib/localContextSource";
import type { ContextPin } from "./ContextHub";
import { call } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import type { ProjectServer } from "../types/api";

/** Workspace, project and stream are the tab's; the reader moves the rest. */
const PINNED: Dimension[] = ["workspace", "project", "stream"];

/**
 * The stream a project is on when its recipe names none.
 *
 * The framework's own default, mirrored here for the case where the venue read
 * has not answered yet or the project binds no venue at all.
 */
const DEFAULT_STREAM = "main";

export interface ContextExplorerViewProps {
  tabID: string;
  /** The open project's name — the ladder's project rung. */
  projectName: string;
  /** Injected in tests and stories; production reads the Wails backend. */
  source?: ContextDataSource;
  /** Injected in tests and stories; production reads the project's venue. */
  stream?: string;
  /** Pin the explorer at a point, and name the rule that sent it there. */
  pin?: ContextPin;
}

/**
 * The context explorer for the open project.
 *
 * The same component set Bowrain renders across a workspace, with the
 * dimensions pinned to this tab: a desktop project is one project on one
 * stream, so the ladder starts at its collections and the panes state the
 * project reach of what they can see.
 */
export function ContextExplorerView({
  tabID,
  projectName,
  source,
  stream,
  pin,
}: ContextExplorerViewProps) {
  const client = useMemo(() => source ?? createLocalContextSource(tabID), [source, tabID]);
  // The stream comes from the project's own venue binding; a project that names
  // none is on the framework's default. Pinning a literal here would make every
  // project read as `main` whether or not it is.
  const server = useQuery({
    queryKey: qk.projectServer(tabID),
    queryFn: () => call<ProjectServer>("GetProjectServer", tabID),
    enabled: stream === undefined && !!tabID,
  });
  const scope = useMemo(
    () => ({
      project: projectName || "project",
      stream: stream ?? server.data?.stream ?? DEFAULT_STREAM,
      coordinate: pin?.coordinate,
      collection: pin?.collection,
      path: pin?.path,
    }),
    [projectName, stream, server.data?.stream, pin?.coordinate, pin?.collection, pin?.path],
  );

  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto p-6">
      <PageHeader
        title="Explorer"
        subtitle="What governs a point in this project, what lives there, and how it relates."
      />
      {pin?.rule && (
        <div
          className="mt-3 flex flex-wrap items-center gap-2 rounded-lg border border-primary/40 bg-primary/5 px-3 py-2 text-xs"
          data-testid="explorer-pin"
        >
          <span className="text-muted-foreground">{t("Pinned by a check finding")}</span>
          <Badge variant="outline" className="font-mono text-[11px] font-normal">
            {pin.rule}
          </Badge>
          {pin.coordinate && (
            <Badge variant="secondary" className="font-normal">
              {pin.coordinate}
            </Badge>
          )}
        </div>
      )}
      <ScopeProvider source={client} pinned={PINNED} scope={scope}>
        <ContextExplorer className="mt-4" />
      </ScopeProvider>
    </div>
  );
}
