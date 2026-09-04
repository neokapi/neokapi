import { useState } from "react";
import { useParams, useRouteContext } from "@tanstack/react-router";
import { Card, AutomationsPage, AutomationRunsPage } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";
import { ProjectFlowsEditor } from "../../flows";
import { usePlatform } from "../../platform";

type Tab = "runs" | "rules" | "flows";

const TABS: { id: Tab; label: string }[] = [
  { id: "runs", label: "Runs" },
  { id: "rules", label: "Rules" },
  { id: "flows", label: "Flows" },
];

/**
 * Superset flow + automation surface for a project. Composes:
 *  - Runs: automation run history (AD-013 run visibility)
 *  - Rules: trigger + conditions + actions; a run_flow action picks a flow
 *  - Flows: the shared linear step editor over the server-side,
 *    connector-agnostic flow definitions that rules reference.
 */
export function AutomationsRoute() {
  const { projectId } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const platform = usePlatform();
  const [tab, setTab] = useState<Tab>("runs");

  if (!activeWorkspace || !projectId) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a project to view automations
      </Card>
    );
  }

  return (
    <div className={`mx-auto w-full p-4 md:p-6 ${tab === "flows" ? "max-w-7xl" : "max-w-5xl"}`}>
      <div className="flex gap-1 mb-4 border-b border-border">
        {TABS.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`px-3 py-1.5 text-sm font-medium border-b-2 transition-colors ${
              tab === t.id
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>
      {/* The run stream is a cookie-authenticated same-origin EventSource, which
          only the web app can open; the desktop app polls the run instead. */}
      {tab === "runs" && (
        <AutomationRunsPage projectId={projectId} live={platform.kind === "web"} />
      )}
      {tab === "rules" && (
        <AutomationsPage workspaceSlug={activeWorkspace.slug} projectId={projectId} />
      )}
      {tab === "flows" && (
        <ProjectFlowsEditor workspaceSlug={activeWorkspace.slug} projectId={projectId} />
      )}
    </div>
  );
}
