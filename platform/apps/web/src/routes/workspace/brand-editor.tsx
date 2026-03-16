import { useEffect, useCallback } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import {
  BrandProfileEditor,
  useBrandProfile,
  useCreateBrandProfile,
  useUpdateBrandProfile,
} from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function BrandEditorRoute() {
  const navigate = useNavigate();
  const { workspace, profileId } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  const isNew = profileId === "new";

  const { data: profile } = useBrandProfile(isNew ? "" : (profileId ?? ""));
  const createMutation = useCreateBrandProfile();
  const updateMutation = useUpdateBrandProfile();

  useEffect(() => {
    const name = isNew ? "New Profile" : (profile?.name ?? "Edit Profile");
    document.title = `${name} — Brand Voice — ${activeWorkspace?.name ?? ""} — Bowrain`;
  }, [isNew, profile?.name, activeWorkspace?.name]);

  const handleCancel = useCallback(() => {
    void navigate({
      to: "/$workspace/brand",
      params: { workspace: workspace ?? "" },
    });
  }, [navigate, workspace]);

  const handleSave = useCallback(
    async (data: Parameters<typeof createMutation.mutateAsync>[0]) => {
      if (isNew) {
        await createMutation.mutateAsync(data);
      } else {
        await updateMutation.mutateAsync({ ...data, id: profileId ?? "" });
      }
      void navigate({
        to: "/$workspace/brand",
        params: { workspace: workspace ?? "" },
      });
    },
    [isNew, profileId, createMutation, updateMutation, navigate, workspace],
  );

  if (!isNew && !profile) {
    return (
      <div className="flex items-center justify-center h-64 text-sm text-muted-foreground">
        Loading profile...
      </div>
    );
  }

  return (
    <BrandProfileEditor
      profile={isNew ? undefined : (profile ?? undefined)}
      onSave={handleSave}
      onCancel={handleCancel}
    />
  );
}
