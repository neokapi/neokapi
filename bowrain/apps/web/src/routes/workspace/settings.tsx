import { useWorkspace, InviteManager, GlassCard, CardContent, type Workspace } from "@gokapi/ui";

function SettingsField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">{label}</div>
      <div className="text-sm text-foreground mt-1">{value}</div>
    </div>
  );
}

export function SettingsIndexRoute() {
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
        <h2 className="text-xl font-semibold">Settings</h2>
        <p className="mt-1 text-[13px] text-muted-foreground">Workspace configuration</p>
      </div>
      <GlassCard intensity="subtle" className="max-w-[480px]">
        <CardContent className="p-6 grid gap-4">
          <SettingsField label="Name" value={activeWorkspace.name} />
          <SettingsField label="Slug" value={activeWorkspace.slug} />
          <SettingsField label="Description" value={activeWorkspace.description || "No description"} />
          <SettingsField label="Your Role" value={activeWorkspace.role} />
        </CardContent>
      </GlassCard>
      <div className="max-w-[480px] mt-4">
        <InviteManager workspace={activeWorkspace} />
      </div>
    </div>
  );
}
