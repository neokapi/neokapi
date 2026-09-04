import { useEffect, useState, useCallback } from "react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  NotificationSettings,
  PageHeader,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  useWorkspace,
  useApi,
  type DigestSettingsDTO,
} from "@neokapi/ui";
import { getStoredUILocale, setUILocale, UI_LOCALES, type UILocale } from "../../i18n";
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
          Share anonymous usage statistics: page views and feature usage, never your content,
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

/**
 * UI-language picker. Persists the choice per browser/app install (see
 * src/i18n.ts) and applies it live — the neokapi-i18n store re-renders the tree
 * from BowrainApp's useNeokapi() subscription. The pseudo locale (qps) is
 * offered only in dev builds for layout and coverage verification.
 */
function LanguageCard() {
  const [locale, setLocale] = useState<UILocale>(() => getStoredUILocale());

  const handleChange = useCallback((next: string) => {
    setLocale(next as UILocale);
    void setUILocale(next as UILocale);
  }, []);

  const options: Array<{ value: UILocale; label: string }> = [...UI_LOCALES];
  if (import.meta.env.DEV || locale === "qps") {
    options.push({ value: "qps", label: "Pseudo (qps)" });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Language</CardTitle>
        <CardDescription>
          Display language for the Bowrain interface. Untranslated text falls back to English.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Select value={locale} onValueChange={handleChange}>
          <SelectTrigger className="w-56" aria-label="Interface language">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {options.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
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
    document.title = "User Settings · Bowrain";
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
      <PageHeader
        title="User Settings"
        subtitle="Profile, handle, and notification preferences for your Bowrain account."
      />

      <ProfileEmailCard />
      <ProfileHandleCard />
      <SecurityCard />
      <LanguageCard />
      {telemetryOptOut && <TelemetryCard optOut={telemetryOptOut} />}

      {settings ? (
        <NotificationSettings settings={settings} onChange={handleChange} saving={saving} />
      ) : (
        <div className="text-sm text-muted-foreground">Loading notification settings…</div>
      )}
    </div>
  );
}
