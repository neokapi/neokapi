import { useParams, useRouteContext } from "@tanstack/react-router";
import { GlassCard, AutomationsPage } from "@gokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function AutomationsRoute() {
  const { projectId } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  if (!activeWorkspace || !projectId) {
    return (
      <GlassCard intensity="subtle" className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a project to view automations
      </GlassCard>
    );
  }

  return (
    <AutomationsPage
      workspaceSlug={activeWorkspace.slug}
      projectId={projectId}
    />
  );
}
