import { useEffect, useState, useCallback } from "react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  NotificationSettings,
  Switch,
  useWorkspace,
  useApi,
  type DigestSettingsDTO,
} from "@neokapi/ui";
import { ProfileEmailCard } from "../../auth/ProfileEmailCard";
import { ProfileHandleCard } from "../../auth/ProfileHandleCard";
import { SecurityCard } from "../../auth/SecurityCard";
import { usePlatform, type PlatformAdapter } from "../../platform";

/**
 * Usage-statistics toggle for local clients (decision D1). Rendered only when
 * the shell exposes the telemetry opt-out through the platform seam (the
 * desktop does; the web omits it — the cloud app is covered by the privacy
 * policy, not a client-side toggle).
 */
type TelemetryOptOut = NonNullable<NonNullable<PlatformAdapter["analytics"]>["optOut"]>;

function TelemetryCard({ optOut }: { optOut: TelemetryOptOut }) {
  const [enabled, setEnabled] = useState(() => optOut.enabled());
  return (
    <Card>
      <CardHeader>
        <CardTitle>Usage statistics</CardTitle>
        <CardDescription>
          Share anonymous usage statistics — page views and feature usage, never your content,
          project names, or file paths.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex items-center justify-between">
        <span className="text-sm">Send anonymous usage statistics</span>
        <Switch
          checked={enabled}
          onCheckedChange={(checked) => {
            optOut.setEnabled(checked);
            setEnabled(checked);
          }}
        />
      </CardContent>
    </Card>
  );
}

export function UserSettingsRoute() {
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
  const platform = usePlatform();
  const telemetryOptOut = platform.analytics?.optOut;
  const [settings, setSettings] = useState<DigestSettingsDTO | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    document.title = "User Settings — Bowrain";
  }, []);

  // Fetch digest settings on mount.
  useEffect(() => {
    if (!activeWorkspace) return;
    let cancelled = false;
    void api.getDigestSettings(activeWorkspace.slug).then((ds) => {
      if (!cancelled) setSettings(ds);
    });
    return () => {
      cancelled = true;
    };
  }, [api, activeWorkspace]);

  const handleChange = useCallback(
    async (updated: DigestSettingsDTO) => {
      if (!activeWorkspace) return;
      setSettings(updated);
      setSaving(true);
      try {
        const saved = await api.updateDigestSettings(activeWorkspace.slug, updated);
        setSettings(saved);
      } finally {
        setSaving(false);
      }
    },
    [api, activeWorkspace],
  );

  return (
    <div className="mx-auto w-full max-w-xl space-y-6 py-4">
      <div>
        <h1 className="text-lg font-semibold">User Settings</h1>
        <p className="text-sm text-muted-foreground">
          Profile, handle, and notification preferences for your Bowrain account.
        </p>
      </div>

      <ProfileEmailCard />
      <ProfileHandleCard />
      <SecurityCard />
      {telemetryOptOut && <TelemetryCard optOut={telemetryOptOut} />}

      {settings ? (
        <NotificationSettings settings={settings} onChange={handleChange} saving={saving} />
      ) : (
        <div className="text-sm text-muted-foreground">Loading notification settings…</div>
      )}
    </div>
  );
}
