import { useState, useEffect, useCallback } from "react";
import {
  useWorkspace,
  useApi,
  BravoConfigPanel,
  BravoUsageDashboard,
  SettingsSkeleton,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
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

  const handleDateRangeChange = useCallback(
    (from: string, to: string) => {
      if (!ws) return;
      void api.bravoGetUsage(ws, from, to).then(setUsage).catch(() => {});
    },
    [api, ws],
  );

  if (!activeWorkspace) return null;

  if (!config) return <SettingsSkeleton />;

  return (
    <div className="mx-auto w-full max-w-3xl py-4 space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Configuration</CardTitle>
          <CardDescription>
            Manage @bravo agent settings and tool permissions for this workspace
          </CardDescription>
        </CardHeader>
        <CardContent>
          <BravoConfigPanel
            config={config}
            tools={tools}
            onSave={(partial: Partial<BravoConfig>) => void handleSave(partial)}
            saving={saving}
          />
        </CardContent>
      </Card>

      {usage && (
        <Card>
          <CardHeader>
            <CardTitle>Usage</CardTitle>
            <CardDescription>
              Token consumption, message volume, and container time
            </CardDescription>
          </CardHeader>
          <CardContent>
            <BravoUsageDashboard
              usage={usage}
              onDateRangeChange={handleDateRangeChange}
            />
          </CardContent>
        </Card>
      )}
    </div>
  );
}
