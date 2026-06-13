import { useEffect, useCallback } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import {
  BrandProfileList,
  BrandProfilesSkeleton,
  useBrandProfiles,
  useDeleteBrandProfile,
} from "@neokapi/ui";
import type { VoiceProfile } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function BrandProfilesRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Brand Voice — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  const { data: profiles, isLoading } = useBrandProfiles();
  const deleteMutation = useDeleteBrandProfile();

  const handleSelect = useCallback(
    (profile: VoiceProfile) => {
      void navigate({
        to: "/$workspace/brand/voice/$profileId",
        params: { workspace: workspace ?? "", profileId: profile.id },
      });
    },
    [navigate, workspace],
  );

  const handleCreate = useCallback(() => {
    void navigate({
      to: "/$workspace/brand/voice/$profileId",
      params: { workspace: workspace ?? "", profileId: "new" },
    });
  }, [navigate, workspace]);

  const handleDelete = useCallback(
    async (profileId: string) => {
      await deleteMutation.mutateAsync(profileId);
    },
    [deleteMutation],
  );

  if (isLoading) {
    return <BrandProfilesSkeleton />;
  }

  return (
    <BrandProfileList
      profiles={profiles ?? []}
      onSelect={handleSelect}
      onCreate={handleCreate}
      onDelete={handleDelete}
    />
  );
}
