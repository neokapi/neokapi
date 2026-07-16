import { useParams, useRouteContext } from "@tanstack/react-router";
import { Card, ConnectorsPanel } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

/**
 * Project-scoped connectors surface. Connectors themselves are workspace-level
 * integrations (WordPress, Figma, HubSpot); this page lists them and binds
 * fetch/publish to the current project.
 */
export function ConnectorsRoute() {
  const { projectId } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  if (!activeWorkspace || !projectId) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a project to manage connectors
      </Card>
    );
  }

  return <ConnectorsPanel workspaceSlug={activeWorkspace.slug} projectId={projectId} />;
}
