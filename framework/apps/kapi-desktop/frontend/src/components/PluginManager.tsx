import { useState, useEffect, useCallback, useRef } from "react";
import { Download, RefreshCw, Search, Package, Loader2, Trash2, ChevronDown, ChevronRight, FileText, Wrench, ArrowUpCircle } from "lucide-react";
import type { PluginInfo } from "../types/api";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";

interface AvailablePlugin {
  name: string;
  version: string;
  description: string;
  type: string;
  installed: boolean;
}

interface PluginUpdate {
  name: string;
  current_version?: string;
  latest_version?: string;
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
  const [updates, setUpdates] = useState<PluginUpdate[]>([]);
  const [checkingUpdates, setCheckingUpdates] = useState(false);
  const [updatingAll, setUpdatingAll] = useState(false);

  const { showError } = useError();

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

  // Refresh installed list when plugins change (install, remove, CLI action).
  useWailsEvent("plugins-changed", () => {
    void loadPlugins();
    if (tab === "available") void loadAvailable();
  });

  // Download progress.
  useWailsEvent("plugin-progress", (data) => {
    const d = data as { percent?: number };
    setInstallStatus((prev) => {
      const next = { ...prev };
      for (const [name, status] of Object.entries(next)) {
        if (status.state === "downloading") {
          next[name] = { ...status, percent: d.percent ?? 0 };
        }
      }
      return next;
    });
  });

  // Install/update complete.
  useWailsEvent("plugin-installed", (data) => {
    const d = data as { name?: string };
    if (d.name) {
      setInstallStatus((prev) => ({ ...prev, [d.name!]: { state: "done" } }));
      // Remove from pending updates list.
      setUpdates((prev) => prev.filter((u) => u.name !== d.name));
      setTimeout(() => {
        setInstallStatus((prev) => {
          const next = { ...prev };
          delete next[d.name!];
          return next;
        });
        // Reset updatingAll when all updates are done.
        setUpdates((prev) => {
          if (prev.length === 0) setUpdatingAll(false);
          return prev;
        });
      }, 2000);
    }
  });

  // Install error.
  useWailsEvent("plugin-error", (data) => {
    const d = data as { name?: string; error?: string };
    if (d.name) {
      setInstallStatus((prev) => ({
        ...prev,
        [d.name!]: { state: "error", error: d.error },
      }));
    }
  });

  const handleInstall = useCallback((name: string) => {
    setInstallStatus((prev) => ({ ...prev, [name]: { state: "downloading", percent: 0 } }));
    // Fire-and-forget — the backend runs in a goroutine and emits events.
    api.installPlugin(name);
  }, []);

  const handleCheckUpdates = useCallback(async () => {
    setError(null);
    setCheckingUpdates(true);
    try {
      const result = await api.checkPluginUpdates();
      if (result && Array.isArray(result) && result.length > 0) {
        setUpdates(result as PluginUpdate[]);
      } else {
        setUpdates([]);
        setError("All plugins are up to date");
      }
    } catch (e) {
      setError(String(e));
    } finally {
      setCheckingUpdates(false);
    }
  }, []);

  const handleUpdate = useCallback((name: string) => {
    setInstallStatus((prev) => ({ ...prev, [name]: { state: "downloading", percent: 0 } }));
    api.updatePlugin(name);
  }, []);

  const handleUpdateAll = useCallback(async () => {
    setUpdatingAll(true);
    for (const u of updates) {
      setInstallStatus((prev) => ({ ...prev, [u.name]: { state: "downloading", percent: 0 } }));
      api.updatePlugin(u.name);
    }
    // The event handlers will manage state from here.
  }, [updates]);

  const handleRemove = useCallback(
    async (pluginId: string) => {
      setRemoving(pluginId);
      setConfirmRemove(null);
      try {
        await api.removePlugin(pluginId);
        // Optimistically remove from local state immediately.
        setPlugins((prev) => prev.filter((p) => p.id !== pluginId));
        setAvailable((prev) => prev.map((p) => p.name === pluginId ? { ...p, installed: false } : p));
      } catch (e) {
        showError("Failed to remove plugin", e);
        // Refresh to restore accurate state on error.
        await loadPlugins();
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
        <div className="flex items-center gap-2">
          {updates.length > 0 && (
            <button
              onClick={handleUpdateAll}
              disabled={updatingAll}
              className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              <ArrowUpCircle size={12} />
              {updatingAll ? "Updating..." : `Update All (${updates.length})`}
            </button>
          )}
          <button
            onClick={handleCheckUpdates}
            disabled={checkingUpdates}
            className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent disabled:opacity-50"
          >
            <RefreshCw size={12} className={checkingUpdates ? "animate-spin" : ""} />
            {checkingUpdates ? "Checking..." : "Check Updates"}
          </button>
        </div>
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
              {filtered.map((plugin) => {
                  const updateInfo = updates.find((u) => u.name === plugin.name);
                  const updateStatus = installStatus[plugin.name];
                  return (
                    <InstalledPluginCard
                      key={plugin.id}
                      plugin={plugin}
                      removing={removing === plugin.id}
                      confirmRemove={confirmRemove === plugin.id}
                      onConfirmRemove={() => setConfirmRemove(plugin.id)}
                      onCancelRemove={() => setConfirmRemove(null)}
                      onRemove={() => void handleRemove(plugin.id)}
                      updateAvailable={updateInfo}
                      updateStatus={updateStatus}
                      onUpdate={() => handleUpdate(plugin.name)}
                    />
                  );
              })}
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

function InstalledPluginCard({
  plugin,
  removing,
  confirmRemove,
  onConfirmRemove,
  onCancelRemove,
  onRemove,
  updateAvailable,
  updateStatus,
  onUpdate,
}: {
  plugin: PluginInfo;
  removing: boolean;
  confirmRemove: boolean;
  onConfirmRemove: () => void;
  onCancelRemove: () => void;
  onRemove: () => void;
  updateAvailable?: PluginUpdate;
  updateStatus?: InstallStatus;
  onUpdate?: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const formatCaps = plugin.capabilities?.filter((c) => c.type === "format") ?? [];
  const toolCaps = plugin.capabilities?.filter((c) => c.type === "tool") ?? [];
  const hasDetails = formatCaps.length > 0 || toolCaps.length > 0;

  return (
    <div className="rounded-lg border border-border overflow-hidden">
      <div className="flex items-center gap-3 p-4">
        <Package size={20} className="shrink-0 text-primary" />
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm font-semibold">{plugin.name}</span>
            {plugin.framework_version && (
              <span className="rounded bg-accent px-1.5 py-0.5 text-xs font-medium">
                {plugin.framework_version}
              </span>
            )}
            <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">v{plugin.version}</span>
            <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{plugin.type}</span>
          </div>
          {plugin.description && (
            <div className="text-xs text-muted-foreground mt-0.5">{plugin.description}</div>
          )}
          {plugin.formats && plugin.formats.length > 0 && (
            <div className="flex flex-wrap gap-1 mt-1.5">
              {plugin.formats.slice(0, 8).map((f) => (
                <span key={f} className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground font-mono">
                  {f}
                </span>
              ))}
              {plugin.formats.length > 8 && (
                <span className="text-[10px] text-muted-foreground">+{plugin.formats.length - 8} more</span>
              )}
            </div>
          )}
          {/* Update progress bar */}
          {updateStatus?.state === "downloading" && updateStatus.percent !== undefined && (
            <div className="mt-2 h-1.5 rounded-full bg-muted overflow-hidden">
              <div
                className="h-full rounded-full bg-primary transition-all duration-300"
                style={{ width: `${updateStatus.percent}%` }}
              />
            </div>
          )}
        </div>
        <div className="flex items-center gap-1 shrink-0">
          {hasDetails && (
            <button
              onClick={() => setExpanded(!expanded)}
              className="p-1.5 rounded hover:bg-accent text-muted-foreground transition-colors"
              title="Show capabilities"
            >
              {expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
            </button>
          )}
          {/* Update button / progress */}
          {updateStatus?.state === "downloading" ? (
            <div className="flex items-center gap-1 text-xs text-primary">
              <Loader2 size={12} className="animate-spin" />
              {updateStatus.percent !== undefined ? `${updateStatus.percent}%` : "Updating..."}
            </div>
          ) : updateStatus?.state === "done" ? (
            <span className="text-[10px] text-primary px-2 py-0.5 rounded bg-primary/10">Updated</span>
          ) : updateStatus?.state === "error" ? (
            <div className="flex items-center gap-1">
              <span className="text-[10px] text-destructive">{updateStatus.error || "Failed"}</span>
              {onUpdate && (
                <button onClick={onUpdate} className="text-[10px] text-primary hover:text-primary/80">Retry</button>
              )}
            </div>
          ) : updateAvailable && onUpdate ? (
            <button
              onClick={onUpdate}
              className="flex items-center gap-1 rounded-md border border-primary/30 bg-primary/5 px-2.5 py-1 text-[10px] font-medium text-primary hover:bg-primary/10 transition-colors"
              title={`Update to v${updateAvailable.latest_version}`}
            >
              <ArrowUpCircle size={11} />
              {updateAvailable.latest_version ? `v${updateAvailable.latest_version}` : "Update"}
            </button>
          ) : null}
          {/* Remove */}
          {removing ? (
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Loader2 size={12} className="animate-spin" /> Removing...
            </div>
          ) : confirmRemove ? (
            <div className="flex items-center gap-1">
              <button onClick={onRemove} className="rounded px-2 py-0.5 text-[10px] bg-destructive text-destructive-foreground">Remove</button>
              <button onClick={onCancelRemove} className="rounded px-2 py-0.5 text-[10px] text-muted-foreground hover:text-foreground">Cancel</button>
            </div>
          ) : (
            <button onClick={onConfirmRemove} className="p-1.5 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors" title="Uninstall">
              <Trash2 size={12} />
            </button>
          )}
        </div>
      </div>

      {/* Expanded capabilities */}
      {expanded && hasDetails && (
        <div className="border-t border-border px-4 py-3">
          {formatCaps.length > 0 && (
            <div className="mb-2">
              <div className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider mb-1 flex items-center gap-1">
                <FileText size={10} /> Formats ({formatCaps.length})
              </div>
              <div className="flex flex-wrap gap-1">
                {formatCaps.map((c) => (
                  <span key={c.name} className="text-[10px] px-1.5 py-0.5 rounded border border-border text-foreground">
                    {c.display_name || c.name}
                    {c.extensions && c.extensions.length > 0 && (
                      <span className="text-muted-foreground ml-1">{c.extensions.join(" ")}</span>
                    )}
                  </span>
                ))}
              </div>
            </div>
          )}
          {toolCaps.length > 0 && (
            <div>
              <div className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider mb-1 flex items-center gap-1">
                <Wrench size={10} /> Tools ({toolCaps.length})
              </div>
              <div className="flex flex-wrap gap-1">
                {toolCaps.map((c) => (
                  <span key={c.name} className="text-[10px] px-1.5 py-0.5 rounded border border-border text-foreground">
                    {c.display_name || c.name}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
