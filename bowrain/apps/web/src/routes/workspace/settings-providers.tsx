import { useWorkspace, GlassCard, CardContent } from "@gokapi/ui";

export function SettingsProvidersRoute() {
  const { activeWorkspace } = useWorkspace();

  if (!activeWorkspace) {
    return (
      <GlassCard intensity="subtle" className="mt-8 max-w-md mx-auto">
        <CardContent className="p-8 text-center text-muted-foreground text-sm">
          Select a workspace
        </CardContent>
      </GlassCard>
    );
  }

  return (
    <div>
      <div className="mb-4">
        <h2 className="text-xl font-semibold">Providers</h2>
        <p className="mt-1 text-[13px] text-muted-foreground">Configure translation and AI providers</p>
      </div>
      <GlassCard intensity="subtle" className="max-w-[480px]">
        <CardContent className="p-6 text-sm text-muted-foreground">
          Provider configuration coming soon.
        </CardContent>
      </GlassCard>
    </div>
  );
}
