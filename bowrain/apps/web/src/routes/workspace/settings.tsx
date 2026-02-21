import { useWorkspace, InviteManager, type Workspace } from "@gokapi/ui";

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
      <div className="mt-8 p-8 text-center text-muted-foreground text-sm rounded-lg border border-dashed border-border">
        Select a workspace
      </div>
    );
  }

  return (
    <div>
      <div className="mb-2">
        <h2 className="text-xl font-semibold">Settings</h2>
        <p className="mt-1 text-[13px] text-muted-foreground">Workspace configuration</p>
      </div>
      <div className="mt-4 grid gap-4 max-w-[480px]">
        <SettingsField label="Name" value={activeWorkspace.name} />
        <SettingsField label="Slug" value={activeWorkspace.slug} />
        <SettingsField label="Description" value={activeWorkspace.description || "No description"} />
        <SettingsField label="Your Role" value={activeWorkspace.role} />
      </div>
      <div className="max-w-[480px]">
        <InviteManager workspace={activeWorkspace} />
      </div>
    </div>
  );
}
