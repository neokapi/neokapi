import { useEffect, useCallback } from "react";
import {
  useWorkspace,
  useApi,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  PulseSettings,
  type DashboardVisibility,
} from "@neokapi/ui";

function SettingsField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="grid grid-cols-3 gap-2 items-baseline py-2.5 border-b border-border/50 last:border-b-0">
      <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
        {label}
      </div>
      <div className={`col-span-2 text-sm text-foreground ${mono ? "font-mono text-xs" : ""}`}>
        {value}
      </div>
    </div>
  );
}

export function SettingsIndexRoute() {
  const { activeWorkspace, setActiveWorkspace } = useWorkspace();
  const adapter = useApi();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Settings — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  const handleVisibilityChange = useCallback(
    async (visibility: DashboardVisibility) => {
      if (!activeWorkspace) return;
      const updated = await adapter.updateWorkspace(activeWorkspace.slug, {
        dashboard_visibility: visibility,
      });
      setActiveWorkspace(updated);
    },
    [activeWorkspace, adapter, setActiveWorkspace],
  );

  if (!activeWorkspace) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </Card>
    );
  }

  const isAdmin = activeWorkspace.role === "owner" || activeWorkspace.role === "admin";

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6 py-4">
      <Card>
        <CardHeader>
          <CardTitle>General</CardTitle>
          <CardDescription>Workspace details and identity</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid">
            <SettingsField label="Name" value={activeWorkspace.name} />
            <SettingsField label="Slug" value={activeWorkspace.slug} />
            <SettingsField
              label="Description"
              value={activeWorkspace.description || "No description"}
            />
            <SettingsField label="Your Role" value={activeWorkspace.role} />
          </div>
        </CardContent>
      </Card>

      {isAdmin && (
        <Card>
          <CardHeader>
            <CardTitle>Pulse Dashboard</CardTitle>
            <CardDescription>Share your localization progress with the community</CardDescription>
          </CardHeader>
          <CardContent>
            <PulseSettings
              workspaceSlug={activeWorkspace.slug}
              visibility={activeWorkspace.dashboard_visibility ?? "private"}
              accessKey={activeWorkspace.pulse_access_key}
              onVisibilityChange={handleVisibilityChange}
            />
          </CardContent>
        </Card>
      )}
    </div>
  );
}
