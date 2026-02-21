import { useWorkspace } from "@gokapi/ui";

export function SettingsProvidersRoute() {
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
        <h2 className="text-xl font-semibold">Providers</h2>
        <p className="mt-1 text-[13px] text-muted-foreground">Configure translation and AI providers</p>
      </div>
      <div className="mt-4 text-sm text-muted-foreground">
        Provider configuration coming soon.
      </div>
    </div>
  );
}
