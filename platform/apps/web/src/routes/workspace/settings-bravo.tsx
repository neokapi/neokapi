import { useState, useEffect, useCallback } from "react";
import {
  useWorkspace,
  useApi,
  BravoConfigPanel,
  SettingsSkeleton,
  type BravoConfig,
  type BravoToolInfo,
  type BravoUsageSummary,
} from "@neokapi/ui";

export function SettingsBravoRoute() {
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
  const ws = activeWorkspace?.slug ?? "";

  const [config, setConfig] = useState<BravoConfig | null>(null);
  const [tools, setTools] = useState<BravoToolInfo[]>([]);
  const [usage, setUsage] = useState<BravoUsageSummary | undefined>();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `@bravo Agent — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  useEffect(() => {
    if (!ws) return;
    void api.bravoGetConfig(ws).then(setConfig).catch(() => {});
    void api.bravoListTools(ws).then((r: { tools: BravoToolInfo[] }) => setTools(r.tools ?? [])).catch(() => {});
    void api.bravoGetUsage(ws).then(setUsage).catch(() => {});
  }, [api, ws]);

  const handleSave = useCallback(
    async (partial: Partial<BravoConfig>) => {
      if (!ws) return;
      setSaving(true);
      try {
        const updated = await api.bravoUpdateConfig(ws, partial);
        setConfig(updated);
      } finally {
        setSaving(false);
      }
    },
    [api, ws],
  );

  if (!activeWorkspace) return null;

  if (!config) return <SettingsSkeleton />;

  return (
    <div className="mx-auto w-full max-w-3xl py-4">
      <BravoConfigPanel
        config={config}
        tools={tools}
        usage={usage}
        onSave={(partial: Partial<BravoConfig>) => void handleSave(partial)}
        saving={saving}
      />
    </div>
  );
}
