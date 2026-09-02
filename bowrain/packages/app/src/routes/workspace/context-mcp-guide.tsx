import { useEffect } from "react";
import { useRouteContext } from "@tanstack/react-router";
import { VoiceMCPGuide } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function ContextMCPGuideRoute() {
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `MCP Guide · Voice · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  return <VoiceMCPGuide serverUrl={window.location.origin} />;
}
