import { useState, useEffect, useCallback, useRef } from "react";
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

interface InstallStatus {
  state: "downloading" | "installing" | "done" | "error";
  percent?: number;
  error?: string;
}

export function PluginManager() {
  const [search, setSearch] = useState("");
  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [available, setAvailable] = useState<AvailablePlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingAvailable, setLoadingAvailable] = useState(false);
  const [installStatus, setInstallStatus] = useState<Record<string, InstallStatus>>({});
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

  // Initial load.
  useEffect(() => {
    void loadPlugins();
  }, [loadPlugins]);

  // Load available when switching to tab.
  useEffect(() => {
    if (tab === "available") {
      void loadAvailable();
    }
  }, [tab, loadAvailable]);

  // Listen for Wails events to stay in sync with CLI and backend.
  useEffect(() => {
    let cleanups: Array<() => void> = [];
    import("@wailsio/runtime")
      .then(({ Events }) => {
        // Refresh installed list when any plugin changes (install, remove, CLI action).
        cleanups.push(
          Events.On("plugins-changed", () => {
            void loadPlugins();
            // Also refresh available list to update installed badges.
            if (tab === "available") void loadAvailable();
          }),
        );

        // Download progress.
        cleanups.push(
          Events.On("plugin-progress", (e: { data: unknown }) => {
            const data = e.data as { percent?: number };
            // Update all currently-installing plugins with progress.
            setInstallStatus((prev) => {
              const next = { ...prev };
              for (const [name, status] of Object.entries(next)) {
                if (status.state === "downloading") {
                  next[name] = { ...status, percent: data.percent ?? 0 };
                }
              }
              return next;
            });
          }),
        );

        // Install complete.
        cleanups.push(
          Events.On("plugin-installed", (e: { data: unknown }) => {
            const data = e.data as { name?: string };
            if (data.name) {
              setInstallStatus((prev) => ({ ...prev, [data.name!]: { state: "done" } }));
              // Clear "done" status after 2 seconds.
              setTimeout(() => {
                setInstallStatus((prev) => {
                  const next = { ...prev };
                  delete next[data.name!];
                  return next;
                });
              }, 2000);
            }
          }),
        );

        // Install error.
        cleanups.push(
          Events.On("plugin-error", (e: { data: unknown }) => {
            const data = e.data as { name?: string; error?: string };
            if (data.name) {
              setInstallStatus((prev) => ({
                ...prev,
                [data.name!]: { state: "error", error: data.error },
              }));
            }
          }),
        );
      })
      .catch(() => {});

    return () => cleanups.forEach((fn) => fn());
  }, [loadPlugins, loadAvailable, tab]);

  const handleInstall = useCallback((name: string) => {
    setInstallStatus((prev) => ({ ...prev, [name]: { state: "downloading", percent: 0 } }));
    // Fire-and-forget — the backend runs in a goroutine and emits events.
    api.installPlugin(name);
  }, []);

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

  const filtered = plugins.filter(
    (p) => !search || p.name.toLowerCase().includes(search.toLowerCase()),
  );

  const searchDebounceRef = useRef<ReturnType<typeof setTimeout>>();
  const handleSearchChange = useCallback(
    (query: string) => {
      setSearch(query);
      clearTimeout(searchDebounceRef.current);
      if (tab === "available") {
        searchDebounceRef.current = setTimeout(() => void loadAvailable(query || undefined), 300);
      }
    },
    [tab, loadAvailable],
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

      {/* Tabs */}
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
          onChange={(e) => handleSearchChange(e.target.value)}
          placeholder={tab === "installed" ? "Filter installed..." : "Search registry..."}
          className="w-full rounded-md border border-input bg-transparent py-2 pl-8 pr-3 text-sm outline-none focus:ring-1 focus:ring-ring"
        />
      </div>

      {error && (
        <p className="mb-4 text-sm text-muted-foreground">{error}</p>
      )}

      {/* Installed tab */}
      {tab === "installed" && (
        <>
          {loading ? (
            <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
              <Loader2 size={16} className="animate-spin" /> Loading plugins...
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
                  {removing === plugin.name ? (
                    <div className="flex items-center gap-1 text-xs text-muted-foreground">
                      <Loader2 size={12} className="animate-spin" /> Removing...
                    </div>
                  ) : confirmRemove === plugin.name ? (
                    <div className="flex items-center gap-1">
                      <button onClick={() => void handleRemove(plugin.name)} className="rounded px-2 py-0.5 text-[10px] bg-destructive text-destructive-foreground">Remove</button>
                      <button onClick={() => setConfirmRemove(null)} className="rounded px-2 py-0.5 text-[10px] text-muted-foreground hover:text-foreground">Cancel</button>
                    </div>
                  ) : (
                    <button onClick={() => setConfirmRemove(plugin.name)} className="p-1.5 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors" title="Uninstall">
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
              <Loader2 size={16} className="animate-spin" /> Loading plugin registry...
            </div>
          ) : (
            <div className="space-y-2">
              {available.map((plugin) => {
                const status = installStatus[plugin.name];
                return (
                  <div key={plugin.name} className="flex items-center gap-3 rounded-lg border border-border p-4">
                    <Package size={20} className={`shrink-0 ${plugin.installed || status?.state === "done" ? "text-primary" : "text-muted-foreground"}`} />
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium">{plugin.name}</span>
                        <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">v{plugin.version}</span>
                        <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{plugin.type}</span>
                      </div>
                      {plugin.description && (
                        <div className="text-xs text-muted-foreground mt-0.5">{plugin.description}</div>
                      )}
                      {/* Download progress bar */}
                      {status?.state === "downloading" && status.percent !== undefined && (
                        <div className="mt-2 h-1.5 rounded-full bg-muted overflow-hidden">
                          <div
                            className="h-full rounded-full bg-primary transition-all duration-300"
                            style={{ width: `${status.percent}%` }}
                          />
                        </div>
                      )}
                    </div>
                    {/* Status / action */}
                    {plugin.installed || status?.state === "done" ? (
                      <span className="text-[10px] text-muted-foreground px-2 py-0.5 rounded bg-muted">Installed</span>
                    ) : status?.state === "downloading" ? (
                      <div className="flex items-center gap-1 text-xs text-muted-foreground">
                        <Loader2 size={12} className="animate-spin" />
                        {status.percent !== undefined ? `${status.percent}%` : "Downloading..."}
                      </div>
                    ) : status?.state === "error" ? (
                      <div className="flex items-center gap-1">
                        <span className="text-[10px] text-destructive">{status.error || "Failed"}</span>
                        <button
                          onClick={() => handleInstall(plugin.name)}
                          className="text-[10px] text-primary hover:text-primary/80"
                        >
                          Retry
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={() => handleInstall(plugin.name)}
                        className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90"
                      >
                        <Download size={12} /> Install
                      </button>
                    )}
                  </div>
                );
              })}
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
