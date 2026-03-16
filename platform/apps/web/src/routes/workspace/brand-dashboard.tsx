import { useEffect } from "react";
import { useRouteContext } from "@tanstack/react-router";
import { BrandDashboard } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function BrandDashboardRoute() {
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Brand Dashboard — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  // Dashboard renders without a specific project selected — show empty state.
  // In future, this will accept a project selector.
  return <BrandDashboard score={null} trends={[]} recentScores={[]} />;
}
