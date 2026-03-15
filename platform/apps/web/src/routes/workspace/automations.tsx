import { useParams, useRouteContext } from "@tanstack/react-router";
import { Card, AutomationsPage } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function AutomationsRoute() {
  const { projectId } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  if (!activeWorkspace || !projectId) {
    return (
      <Card
        className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm"
      >
        Select a project to view automations
      </Card>
    );
  }

  return (
    <div className="mx-auto w-full max-w-5xl p-4 md:p-6">
      <AutomationsPage workspaceSlug={activeWorkspace.slug} projectId={projectId} />
    </div>
  );
}
