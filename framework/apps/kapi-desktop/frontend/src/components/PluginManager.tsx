import { useState, useEffect, useCallback } from "react";
import { Download, RefreshCw, Search, Package, Loader2 } from "lucide-react";
import type { PluginInfo } from "../types/api";
import { api } from "../hooks/useApi";

export function PluginManager() {
  const [search, setSearch] = useState("");
  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [installing, setInstalling] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const loadPlugins = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await api.listPlugins();
      if (result) setPlugins(result);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadPlugins();
  }, [loadPlugins]);

  const handleInstall = useCallback(
    (name: string) => {
      setInstalling(name);
      setError(null);
      api.installPlugin(name);
      // InstallPlugin is async on the backend — it emits events.
      // For now, refresh after a delay. TODO: listen to plugin-installed event.
      setTimeout(() => {
        setInstalling(null);
        loadPlugins();
      }, 3000);
    },
    [loadPlugins],
  );

  const handleCheckUpdates = useCallback(async () => {
    setError(null);
    try {
      const updates = await api.checkPluginUpdates();
      if (updates && Array.isArray(updates) && updates.length > 0) {
        setError(`${updates.length} update(s) available`);
      } else {
        setError("All plugins are up to date");
      }
    } catch (e) {
      setError(String(e));
    }
  }, []);

  const filtered = plugins.filter(
    (p) => p.name.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Plugins</h1>
        <button
          onClick={handleCheckUpdates}
          className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent"
          aria-label="Check for plugin updates"
        >
          <RefreshCw size={12} />
          Check Updates
        </button>
      </div>

      <div className="relative mb-4">
        <Search size={14} className="absolute left-2.5 top-2.5 text-muted-foreground" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search plugins..."
          className="w-full rounded-md border border-input bg-transparent py-2 pl-8 pr-3 text-sm outline-none focus:ring-1 focus:ring-ring"
          aria-label="Search plugins"
        />
      </div>

      {error && (
        <p className="mb-4 text-sm text-muted-foreground" role="status">{error}</p>
      )}

      {loading ? (
        <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Loader2 size={16} className="animate-spin" />
          Loading plugins...
        </div>
      ) : (
        <div className="space-y-2">
          {filtered.map((plugin) => (
            <div
              key={plugin.name}
              className="flex items-center gap-3 rounded-lg border border-border p-4"
            >
              <Package size={20} className="shrink-0 text-primary" />
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{plugin.name}</span>
                  <span className="rounded bg-accent px-1.5 py-0.5 text-xs">
                    v{plugin.version}
                  </span>
                  <span className="rounded bg-accent px-1.5 py-0.5 text-xs">
                    {plugin.type}
                  </span>
                </div>
              </div>
              {installing === plugin.name ? (
                <div className="flex items-center gap-1 text-xs text-muted-foreground">
                  <Loader2 size={12} className="animate-spin" />
                  Installing...
                </div>
              ) : (
                <span className="text-xs text-muted-foreground">Installed</span>
              )}
            </div>
          ))}
          {filtered.length === 0 && (
            <p className="py-8 text-center text-sm text-muted-foreground">
              No plugins found
            </p>
          )}
        </div>
      )}
    </div>
  );
}
