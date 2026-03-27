import { useState } from "react";
import { useParams, useRouteContext } from "@tanstack/react-router";
import { Card, AutomationsPage, AutomationRunsPage } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function AutomationsRoute() {
  const { projectId } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const [tab, setTab] = useState<"rules" | "runs">("runs");

  if (!activeWorkspace || !projectId) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a project to view automations
      </Card>
    );
  }

  return (
    <div className="mx-auto w-full max-w-5xl p-4 md:p-6">
      <div className="flex gap-1 mb-4 border-b border-border">
        <button
          onClick={() => setTab("runs")}
          className={`px-3 py-1.5 text-sm font-medium border-b-2 transition-colors ${
            tab === "runs"
              ? "border-primary text-foreground"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          Runs
        </button>
        <button
          onClick={() => setTab("rules")}
          className={`px-3 py-1.5 text-sm font-medium border-b-2 transition-colors ${
            tab === "rules"
              ? "border-primary text-foreground"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          Rules
        </button>
      </div>
      {tab === "runs" ? (
        <AutomationRunsPage projectId={projectId} />
      ) : (
        <AutomationsPage workspaceSlug={activeWorkspace.slug} projectId={projectId} />
      )}
    </div>
  );
}
