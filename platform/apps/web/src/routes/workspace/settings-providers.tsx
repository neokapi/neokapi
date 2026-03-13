import { useWorkspace, GlassCard } from "@neokapi/ui";

export function SettingsProvidersRoute() {
  const { activeWorkspace } = useWorkspace();

  if (!activeWorkspace) {
    return (
      <GlassCard
        intensity="subtle"
        className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm"
      >
        Select a workspace
      </GlassCard>
    );
  }

  return (
    <div className="max-w-[560px]">
      <GlassCard intensity="subtle" className="p-6">
        <div className="mb-6">
          <h2 className="text-xl font-semibold">Providers</h2>
          <p className="mt-1 text-[13px] text-muted-foreground">
            Configure translation and AI providers
          </p>
        </div>
        <div className="py-8 text-center text-sm text-muted-foreground">
          Provider configuration coming soon.
        </div>
      </GlassCard>
    </div>
  );
}
