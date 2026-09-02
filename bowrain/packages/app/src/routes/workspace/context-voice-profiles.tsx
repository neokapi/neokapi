import { useEffect, useCallback } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import {
  VoiceProfileList,
  VoiceProfilesSkeleton,
  useVoiceProfiles,
  useDeleteVoiceProfile,
} from "@neokapi/ui";
import type { VoiceProfile } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function ContextVoiceProfilesRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace, contextScanAvailable } = useRouteContext({
    strict: false,
  }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Voice · Context · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const { data: profiles, isLoading } = useVoiceProfiles();
  const deleteMutation = useDeleteVoiceProfile();

  const handleSelect = useCallback(
    (profile: VoiceProfile) => {
      void navigate({
        to: "/$workspace/context/voice/$profileId",
        params: { workspace: workspace ?? "", profileId: profile.id },
      });
    },
    [navigate, workspace],
  );

  const handleCreate = useCallback(() => {
    void navigate({
      to: "/$workspace/context/voice/$profileId",
      params: { workspace: workspace ?? "", profileId: "new" },
    });
  }, [navigate, workspace]);

  const handleScanVoice = useCallback(() => {
    void navigate({
      to: "/$workspace/context/scan",
      params: { workspace: workspace ?? "" },
    });
  }, [navigate, workspace]);

  const handleLocalLane = useCallback(() => {
    void navigate({
      to: "/$workspace/context/voice/mcp-guide",
      params: { workspace: workspace ?? "" },
    });
  }, [navigate, workspace]);

  const handleReview = useCallback(
    (profileId: string) => {
      void navigate({
        to: "/$workspace/context/voice/review/$profileId",
        params: { workspace: workspace ?? "", profileId },
      });
    },
    [navigate, workspace],
  );

  const handleDelete = useCallback(
    async (profileId: string) => {
      await deleteMutation.mutateAsync(profileId);
    },
    [deleteMutation],
  );

  if (isLoading) {
    return <VoiceProfilesSkeleton />;
  }

  return (
    <VoiceProfileList
      profiles={profiles ?? []}
      onSelect={handleSelect}
      onCreate={handleCreate}
      onDelete={handleDelete}
      onReview={handleReview}
      // Only servers running the brand-scan job system get the hosted-scan
      // CTA; the local lane (kapi Agent Skill) is always available.
      onScanVoice={contextScanAvailable ? handleScanVoice : undefined}
      onLocalLane={handleLocalLane}
    />
  );
}
