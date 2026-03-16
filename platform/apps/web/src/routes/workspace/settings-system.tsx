import { useState, useEffect } from "react";
import {
  useWorkspace,
  useApi,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  SettingsSkeleton,
  type ConfigResponse,
  type WebVersionInfo,
} from "@neokapi/ui";

function InfoField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
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

export function SettingsSystemRoute() {
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
  const [serverInfo, setServerInfo] = useState<ConfigResponse | null>(null);
  const [webInfo, setWebInfo] = useState<WebVersionInfo | null>(null);

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `System Info — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  useEffect(() => {
    api
      .getConfig()
      .then(setServerInfo)
      .catch(() => {});
    fetch("/version.json")
      .then((r) => (r.ok ? r.json() : null))
      .then(setWebInfo)
      .catch(() => {});
  }, [api]);

  if (!activeWorkspace) return null;

  return (
    <div className="mx-auto w-full max-w-3xl py-4 space-y-4">
      {serverInfo && (
        <Card>
          <CardHeader>
            <CardTitle>Server</CardTitle>
            <CardDescription>Bowrain server build information</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid">
              <InfoField label="Version" value={serverInfo.version} />
              <InfoField label="Commit" value={serverInfo.commit} mono />
              <InfoField label="Build Date" value={serverInfo.build_date} />
            </div>
          </CardContent>
        </Card>
      )}
      {webInfo && (
        <Card>
          <CardHeader>
            <CardTitle>Web App</CardTitle>
            <CardDescription>Frontend build information</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid">
              <InfoField label="Version" value={webInfo.version} />
              <InfoField label="Commit" value={webInfo.commit} mono />
              <InfoField label="Build Date" value={webInfo.build_date} />
            </div>
          </CardContent>
        </Card>
      )}
      {!serverInfo && !webInfo && <SettingsSkeleton />}
    </div>
  );
}
