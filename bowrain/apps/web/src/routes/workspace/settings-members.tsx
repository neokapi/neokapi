import { useWorkspace, GlassCard } from "@gokapi/ui";

export function SettingsMembersRoute() {
  const { activeWorkspace } = useWorkspace();

  if (!activeWorkspace) {
    return (
      <GlassCard intensity="subtle" className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </GlassCard>
    );
  }

  return (
    <div className="max-w-[560px]">
      <GlassCard intensity="subtle" className="p-6">
        <div className="mb-6">
          <h2 className="text-xl font-semibold">Members</h2>
          <p className="mt-1 text-[13px] text-muted-foreground">Manage workspace members</p>
        </div>
        <div className="py-8 text-center text-sm text-muted-foreground">
          Member management coming soon.
        </div>
      </GlassCard>
    </div>
  );
}
