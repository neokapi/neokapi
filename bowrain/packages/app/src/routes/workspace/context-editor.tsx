import { useState, useEffect, useCallback } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import {
  VoiceProfileWizard,
  StarterPackPicker,
  ErrorNotice,
  useVoiceProfile,
  useCreateVoiceProfile,
  useUpdateVoiceProfile,
  useCreateFromStarter,
  useAnalytics,
  AnalyticsEvents,
} from "@neokapi/ui";
import type { StarterPackMeta } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function ContextEditorRoute() {
  const navigate = useNavigate();
  const { workspace, profileId } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({
    strict: false,
  }) as WorkspaceRouteContext;

  const isNew = profileId === "new";
  const [showPicker, setShowPicker] = useState(false);
  const [starterError, setStarterError] = useState<unknown>(null);

  // Show picker on first render for new profiles (triggered via "From Starter" button)
  const [pickerShownOnce, setPickerShownOnce] = useState(false);

  const { data: profile } = useVoiceProfile(isNew ? "" : (profileId ?? ""));
  const createMutation = useCreateVoiceProfile();
  const updateMutation = useUpdateVoiceProfile();
  const createFromStarter = useCreateFromStarter();
  const { capture } = useAnalytics();

  useEffect(() => {
    const name = isNew ? "New Profile" : (profile?.name ?? "Edit Profile");
    document.title = `${name} · Voice · ${activeWorkspace?.name ?? ""} · Bowrain`;
  }, [isNew, profile?.name, activeWorkspace?.name]);

  // Auto-show picker for new profiles on initial load
  useEffect(() => {
    if (isNew && !pickerShownOnce) {
      setShowPicker(true);
      setPickerShownOnce(true);
    }
  }, [isNew, pickerShownOnce]);

  const handleCancel = useCallback(() => {
    void navigate({
      to: "/$workspace/context/voice",
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
      capture(AnalyticsEvents.brandVoiceSaved, { mode: isNew ? "created" : "updated" });
      void navigate({
        to: "/$workspace/context/voice",
        params: { workspace: workspace ?? "" },
      });
    },
    [isNew, profileId, createMutation, updateMutation, capture, navigate, workspace],
  );

  // Selecting a starter pack creates the profile from the server's authoritative
  // template (core/profile/packs/*.yaml) and opens it in the editor for review —
  // no client-side copy of the pack content to drift from the source.
  const handleSelectPack = useCallback(
    async (pack: StarterPackMeta) => {
      setStarterError(null);
      try {
        const created = await createFromStarter.mutateAsync({
          pack: pack.name,
          name: pack.label,
        });
        capture(AnalyticsEvents.brandVoiceSaved, { mode: "created", source: "starter" });
        setShowPicker(false);
        void navigate({
          to: "/$workspace/context/voice/$profileId",
          params: { workspace: workspace ?? "", profileId: created.id },
        });
      } catch (err) {
        setStarterError(err);
      }
    },
    [createFromStarter, capture, navigate, workspace],
  );

  const handleScratch = useCallback(() => {
    setShowPicker(false);
  }, []);

  if (!isNew && !profile) {
    return (
      <div className="flex items-center justify-center h-64 text-sm text-muted-foreground">
        Loading profile...
      </div>
    );
  }

  const wizardProfile = isNew ? undefined : (profile ?? undefined);

  return (
    <>
      <StarterPackPicker
        open={showPicker}
        onOpenChange={setShowPicker}
        onSelect={handleSelectPack}
        onScratch={handleScratch}
        busy={createFromStarter.isPending}
      />
      {starterError != null && (
        <div className="mx-auto max-w-2xl px-4 pt-4">
          <ErrorNotice error={starterError} title="Could not create the profile from that pack" />
        </div>
      )}
      <VoiceProfileWizard profile={wizardProfile} onSave={handleSave} onCancel={handleCancel} />
    </>
  );
}
