import { useCallback, useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { ProfileDetailView } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

/** One governance profile: its point, voice, vocabulary, content and changes. */
export function ContextProfileDetailRoute() {
  const navigate = useNavigate();
  const { workspace, slug } = useParams({ strict: false });
  const { activeWorkspace, contextScanAvailable } = useRouteContext({
    strict: false,
  }) as WorkspaceRouteContext;

  const ws = workspace ?? "";
  const profileSlug = slug ?? "";

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `${profileSlug} · Profiles · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace, profileSlug]);

  const go = useCallback(
    (to: string, params: Record<string, string> = {}) => {
      void navigate({ to, params: { workspace: ws, ...params } });
    },
    [navigate, ws],
  );

  return (
    <ProfileDetailView
      slug={profileSlug}
      onBack={() => go("/$workspace/context/profiles")}
      onOpenVoice={(profileId) => go("/$workspace/context/voice/$profileId", { profileId })}
      onOpenTerms={() => go("/$workspace/context/concepts")}
      onOpenChanges={() => go("/$workspace/context/changes")}
      onScanVoice={contextScanAvailable ? () => go("/$workspace/context/scan") : undefined}
    />
  );
}
