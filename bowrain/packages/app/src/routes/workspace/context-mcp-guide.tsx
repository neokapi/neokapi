import { useEffect } from "react";
import { useRouteContext } from "@tanstack/react-router";
import { VoiceMCPGuide, PageContainer } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function ContextMCPGuideRoute() {
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `MCP Guide · Voice · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  return (
    <PageContainer width="standard">
      <VoiceMCPGuide serverUrl={window.location.origin} />
    </PageContainer>
  );
}
