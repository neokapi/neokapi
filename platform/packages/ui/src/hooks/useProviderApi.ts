import { useState, useEffect, useCallback } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { ProviderConfig, ProviderConfigWithKey } from "../types/api";

export function useProviderConfigs() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const [configs, setConfigs] = useState<ProviderConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(() => {
    if (!ws) return;
    setLoading(true);
    api.listProviderConfigs(ws)
      .then((c) => setConfigs(c || []))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [api, ws]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { configs, loading, error, refresh };
}

export function useProviderApi() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const saveProviderConfig = useCallback(
    async (cfg: ProviderConfigWithKey): Promise<ProviderConfig> =>
      api.saveProviderConfig(ws, cfg),
    [api, ws],
  );

  const deleteProviderConfig = useCallback(
    async (id: string): Promise<void> => api.deleteProviderConfig(ws, id),
    [api, ws],
  );

  const testProviderConfig = useCallback(
    async (cfg: ProviderConfigWithKey): Promise<void> => api.testProviderConfig(ws, cfg),
    [api, ws],
  );

  return { saveProviderConfig, deleteProviderConfig, testProviderConfig };
}
