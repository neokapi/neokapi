import { useState, useEffect, useCallback } from "react";
import { Download, RefreshCw, Search, Package, Loader2, Trash2 } from "lucide-react";
import type { PluginInfo } from "../types/api";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";

interface AvailablePlugin {
  name: string;
  version: string;
  description: string;
  type: string;
  installed: boolean;
}

export function PluginManager() {
  const [search, setSearch] = useState("");
  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [available, setAvailable] = useState<AvailablePlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingAvailable, setLoadingAvailable] = useState(false);
  const [installing, setInstalling] = useState<string | null>(null);
  const [removing, setRemoving] = useState<string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<"installed" | "available">("installed");

  const { showError } = useError();

  const loadPlugins = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await api.listPlugins();
      if (result) {
        // Deduplicate by name (plugin loader may return duplicates after re-scan).
        const seen = new Set<string>();
        const deduped = result.filter((p) => {
          if (seen.has(p.name)) return false;
          seen.add(p.name);
          return true;
        });
        setPlugins(deduped);
      }
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
    async (name: string) => {
      setInstalling(name);
      setError(null);
      try {
        await api.installPlugin(name);
        await loadPlugins();
      } catch (e) {
        setError(String(e));
      } finally {
        setInstalling(null);
      }
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

  const loadAvailable = useCallback(async (query?: string) => {
    setLoadingAvailable(true);
    try {
      const result = query
        ? await api.searchPlugins(query)
        : await api.listAvailablePlugins();
      if (result) setAvailable(result as AvailablePlugin[]);
    } catch (e) {
      showError("Failed to load available plugins", e);
    } finally {
      setLoadingAvailable(false);
    }
  }, [showError]);

  // Load available plugins when switching to the tab.
  useEffect(() => {
    if (tab === "available" && available.length === 0) {
      void loadAvailable();
    }
  }, [tab, available.length, loadAvailable]);

  const handleSearch = useCallback(async (query: string) => {
    if (tab === "available" && query.trim()) {
      await loadAvailable(query);
    }
  }, [tab, loadAvailable]);

  const handleRemove = useCallback(
    async (name: string) => {
      setRemoving(name);
      setConfirmRemove(null);
      try {
        await api.removePlugin(name);
        await loadPlugins();
      } catch (e) {
        showError("Failed to remove plugin", e);
      } finally {
        setRemoving(null);
      }
    },
    [loadPlugins, showError],
  );

  const filtered = plugins.filter((p) => p.name.toLowerCase().includes(search.toLowerCase()));

  const searchDebounceRef = useCallback(
    (() => {
      let timer: ReturnType<typeof setTimeout>;
      return (query: string) => {
        clearTimeout(timer);
        if (tab === "available") {
          timer = setTimeout(() => void handleSearch(query), 300);
        }
      };
    })(),
    [tab, handleSearch],
  );

  return (
    <div className="p-6">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Plugins</h1>
        <button
          onClick={handleCheckUpdates}
          className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent"
        >
          <RefreshCw size={12} />
          Check Updates
        </button>
      </div>

      {/* Installed / Available tabs */}
      <div className="flex gap-0 mb-4 border-b border-border">
        <button
          onClick={() => setTab("installed")}
          className={`px-4 py-2 text-xs font-medium transition-colors ${
            tab === "installed"
              ? "border-b-2 border-primary text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          Installed ({plugins.length})
        </button>
        <button
          onClick={() => setTab("available")}
          className={`px-4 py-2 text-xs font-medium transition-colors ${
            tab === "available"
              ? "border-b-2 border-primary text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          Available
        </button>
      </div>

      <div className="relative mb-4">
        <Search size={14} className="absolute left-2.5 top-2.5 text-muted-foreground" />
        <input
          type="text"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            searchDebounceRef(e.target.value);
          }}
          placeholder={tab === "installed" ? "Filter installed..." : "Search registry..."}
          className="w-full rounded-md border border-input bg-transparent py-2 pl-8 pr-3 text-sm outline-none focus:ring-1 focus:ring-ring"
        />
      </div>

      {error && (
        <p className="mb-4 text-sm text-muted-foreground" role="status">{error}</p>
      )}

      {/* Installed tab */}
      {tab === "installed" && (
        <>
          {loading ? (
            <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
              <Loader2 size={16} className="animate-spin" />
              Loading plugins...
            </div>
          ) : (
            <div className="space-y-2">
              {filtered.map((plugin) => (
                <div key={plugin.name} className="flex items-center gap-3 rounded-lg border border-border p-4">
                  <Package size={20} className="shrink-0 text-primary" />
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{plugin.name}</span>
                      <span className="rounded bg-accent px-1.5 py-0.5 text-xs">v{plugin.version}</span>
                      <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{plugin.type}</span>
                    </div>
                  </div>
                  {installing === plugin.name || removing === plugin.name ? (
                    <div className="flex items-center gap-1 text-xs text-muted-foreground">
                      <Loader2 size={12} className="animate-spin" />
                      {removing === plugin.name ? "Removing..." : "Updating..."}
                    </div>
                  ) : confirmRemove === plugin.name ? (
                    <div className="flex items-center gap-1">
                      <button onClick={() => void handleRemove(plugin.name)} className="rounded px-2 py-0.5 text-[10px] bg-destructive text-destructive-foreground">Remove</button>
                      <button onClick={() => setConfirmRemove(null)} className="rounded px-2 py-0.5 text-[10px] text-muted-foreground hover:text-foreground">Cancel</button>
                    </div>
                  ) : (
                    <button
                      onClick={() => setConfirmRemove(plugin.name)}
                      className="p-1.5 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors"
                      title="Uninstall plugin"
                    >
                      <Trash2 size={12} />
                    </button>
                  )}
                </div>
              ))}
              {filtered.length === 0 && (
                <div className="py-8 text-center">
                  <p className="text-sm text-muted-foreground mb-2">No plugins installed.</p>
                  <button onClick={() => setTab("available")} className="text-xs text-primary hover:text-primary/80">
                    Browse available plugins
                  </button>
                </div>
              )}
            </div>
          )}
        </>
      )}

      {/* Available tab */}
      {tab === "available" && (
        <>
          {loadingAvailable ? (
            <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
              <Loader2 size={16} className="animate-spin" />
              Loading plugin registry...
            </div>
          ) : (
            <div className="space-y-2">
              {available.map((plugin) => (
                <div key={plugin.name} className="flex items-center gap-3 rounded-lg border border-border p-4">
                  <Package size={20} className={`shrink-0 ${plugin.installed ? "text-primary" : "text-muted-foreground"}`} />
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{plugin.name}</span>
                      <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">v{plugin.version}</span>
                      <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{plugin.type}</span>
                    </div>
                    {plugin.description && (
                      <div className="text-xs text-muted-foreground mt-0.5">{plugin.description}</div>
                    )}
                  </div>
                  {plugin.installed ? (
                    <span className="text-[10px] text-muted-foreground px-2 py-0.5 rounded bg-muted">Installed</span>
                  ) : installing === plugin.name ? (
                    <div className="flex items-center gap-1 text-xs text-muted-foreground">
                      <Loader2 size={12} className="animate-spin" />
                      Installing...
                    </div>
                  ) : (
                    <button
                      onClick={() => void handleInstall(plugin.name)}
                      className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90"
                    >
                      <Download size={12} />
                      Install
                    </button>
                  )}
                </div>
              ))}
              {available.length === 0 && (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  {search ? "No plugins match your search." : "No plugins available."}
                </p>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
