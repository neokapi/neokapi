import { useState, useEffect } from "react";
import { useWorkspace, useApi, GlassCard, InviteManager, ApiTokenManager, type ConfigResponse, type WebVersionInfo } from "@neokapi/ui";

function SettingsField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">{label}</div>
      <div className={`text-sm text-foreground mt-1 ${mono ? "font-mono text-xs" : ""}`}>{value}</div>
    </div>
  );
}

function VersionSection() {
  const api = useApi();
  const [serverInfo, setServerInfo] = useState<ConfigResponse | null>(null);
  const [webInfo, setWebInfo] = useState<WebVersionInfo | null>(null);

  useEffect(() => {
    api.getConfig().then(setServerInfo).catch(() => {});
    fetch("/version.json")
      .then((r) => (r.ok ? r.json() : null))
      .then(setWebInfo)
      .catch(() => {});
  }, [api]);

  if (!serverInfo && !webInfo) return null;

  return (
    <GlassCard intensity="subtle" className="p-6">
      <div className="mb-6">
        <h2 className="text-xl font-semibold">Version</h2>
        <p className="mt-1 text-[13px] text-muted-foreground">Build information</p>
      </div>
      <div className="grid gap-4">
        {serverInfo && (
          <>
            <SettingsField label="Server Version" value={serverInfo.version} />
            <SettingsField label="Server Commit" value={serverInfo.commit} mono />
            <SettingsField label="Server Build Date" value={serverInfo.build_date} />
          </>
        )}
        {webInfo && (
          <>
            <SettingsField label="Web Version" value={webInfo.version} />
            <SettingsField label="Web Commit" value={webInfo.commit} mono />
            <SettingsField label="Web Build Date" value={webInfo.build_date} />
          </>
        )}
      </div>
    </GlassCard>
  );
}

export function SettingsIndexRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Settings — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) {
    return (
      <GlassCard intensity="subtle" className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </GlassCard>
    );
  }

  return (
    <div className="space-y-4 max-w-[560px]">
      <GlassCard intensity="subtle" className="p-6">
        <div className="mb-6">
          <h2 className="text-xl font-semibold">Settings</h2>
          <p className="mt-1 text-[13px] text-muted-foreground">Workspace configuration</p>
        </div>
        <div className="grid gap-4">
          <SettingsField label="Name" value={activeWorkspace.name} />
          <SettingsField label="Slug" value={activeWorkspace.slug} />
          <SettingsField label="Description" value={activeWorkspace.description || "No description"} />
          <SettingsField label="Your Role" value={activeWorkspace.role} />
        </div>
      </GlassCard>
      <InviteManager workspace={activeWorkspace} />
      <ApiTokenManager workspace={activeWorkspace} />
      <VersionSection />
    </div>
  );
}
