import { useCallback, useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { ProfilesView } from "@neokapi/ui";
import { usePlatform } from "../../platform";
import type { WorkspaceRouteContext } from "..";

/** The Context hub's landing: every point the workspace's content sits at. */
export function ContextGovernanceProfilesRoute() {
  const navigate = useNavigate();
  const platform = usePlatform();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace, contextScanAvailable } = useRouteContext({
    strict: false,
  }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Profiles · Context · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const ws = workspace ?? "";

  const handleOpenProfile = useCallback(
    (slug: string) => {
      void navigate({
        to: "/$workspace/context/profiles/$slug",
        params: { workspace: ws, slug },
      });
    },
    [navigate, ws],
  );

  const handleScanVoice = useCallback(() => {
    void navigate({ to: "/$workspace/context/scan", params: { workspace: ws } });
  }, [navigate, ws]);

  return (
    <ProfilesView
      onOpenProfile={handleOpenProfile}
      // Only servers running the brand-scan job system get the hosted-scan
      // action; the local lane (kapi Agent Skill) is always available.
      onScanVoice={contextScanAvailable ? handleScanVoice : undefined}
      serverUrl={platform.kind === "web" ? window.location.origin : undefined}
    />
  );
}
