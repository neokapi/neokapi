import { useEffect } from "react";
import { useRouteContext } from "@tanstack/react-router";
import { BrandMCPGuide } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function BrandMCPGuideRoute() {
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `MCP Guide — Brand Voice — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  return <BrandMCPGuide />;
}
