import { useEffect, useState, useCallback } from "react";
import { NotificationSettings, useWorkspace, useApi, type DigestSettingsDTO } from "@neokapi/ui";

export function UserSettingsRoute() {
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
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
    return () => { cancelled = true; };
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

  if (!settings) {
    return (
      <div className="mx-auto w-full max-w-xl py-8 text-sm text-muted-foreground text-center">
        Loading settings...
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-xl space-y-6 py-4">
      <div>
        <h1 className="text-lg font-semibold">User Settings</h1>
        <p className="text-sm text-muted-foreground">
          Notification preferences for this workspace.
        </p>
      </div>
      <NotificationSettings settings={settings} onChange={handleChange} saving={saving} />
    </div>
  );
}
