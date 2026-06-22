import { useState, useEffect, useCallback, useRef } from "react";
import {
  Download,
  RefreshCw,
  Search,
  Package,
  Loader2,
  Trash2,
  ChevronDown,
  ChevronRight,
  FileText,
  Wrench,
  ArrowUpCircle,
} from "lucide-react";
import {
  Button,
  Badge,
  ItemCard,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  LoadingSpinner,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
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
  /** False when the registry has no build of this plugin for the running OS/arch. */
  available: boolean;
  /** Running OS/arch ("windows/arm64"), shown when the plugin is unavailable. */
  platform: string;
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

export interface PluginManagerProps {
  /** Pre-loaded installed plugins for Storybook — skips api.listPlugins(). */
  plugins?: PluginInfo[];
}

export function PluginManager({ plugins: propPlugins }: PluginManagerProps = {}) {
  const [search, setSearch] = useState("");
  const [plugins, setPlugins] = useState<PluginInfo[]>(propPlugins ?? []);
  const [available, setAvailable] = useState<AvailablePlugin[]>([]);
  const [loading, setLoading] = useState(!propPlugins);
  const [loadingAvailable, setLoadingAvailable] = useState(false);
  const [installStatus, setInstallStatus] = useState<Record<string, InstallStatus>>({});
  const [removing, setRemoving] = useState<string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<string>("installed");
  const [updates, setUpdates] = useState<PluginUpdate[]>([]);
  const [checkingUpdates, setCheckingUpdates] = useState(false);
  const [updatingAll, setUpdatingAll] = useState(false);

  const { showError } = useError();

  const loadPlugins = useCallback(async () => {
    if (propPlugins) return;
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
  }, [propPlugins]);

  const loadAvailable = useCallback(
    async (query?: string) => {
      setLoadingAvailable(true);
      try {
        const result = query ? await api.searchPlugins(query) : await api.listAvailablePlugins();
        if (result) setAvailable(result as AvailablePlugin[]);
      } catch (e) {
        showError("Failed to load available plugins", e);
      } finally {
        setLoadingAvailable(false);
      }
    },
    [showError],
  );

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
    void api.installPlugin(name);
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
    void api.updatePlugin(name);
  }, []);

  const handleUpdateAll = useCallback(async () => {
    setUpdatingAll(true);
    for (const u of updates) {
      setInstallStatus((prev) => ({ ...prev, [u.name]: { state: "downloading", percent: 0 } }));
      void api.updatePlugin(u.name);
    }
    // The event handlers will manage state from here.
  }, [updates]);

  const handleRemove = useCallback(
    // pluginId keys the per-row UI state; pluginName is what the backend
    // uninstall resolves against. Passing the composite id here was the cause
    // of the failed-uninstall crash (issue #8).
    async (pluginId: string, pluginName: string) => {
      setRemoving(pluginId);
      setConfirmRemove(null);
      try {
        await api.removePlugin(pluginName);
        // Optimistically remove from local state immediately.
        setPlugins((prev) => prev.filter((p) => p.id !== pluginId));
        setAvailable((prev) =>
          prev.map((p) => (p.name === pluginName ? { ...p, installed: false } : p)),
        );
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

  const searchDebounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);
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
            <Button size="sm" onClick={handleUpdateAll} disabled={updatingAll}>
              <ArrowUpCircle size={12} />
              {updatingAll
                ? t("Updating...")
                : t("Update All ({count})", { count: updates.length })}
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={handleCheckUpdates}
            disabled={checkingUpdates}
          >
            <RefreshCw size={12} className={checkingUpdates ? "animate-spin" : ""} />
            {checkingUpdates ? t("Checking...") : t("Check Updates")}
          </Button>
        </div>
      </div>

      <Tabs defaultValue="installed" onValueChange={setTab}>
        <TabsList variant="line" className="mb-4">
          <TabsTrigger value="installed">Installed ({plugins.length})</TabsTrigger>
          <TabsTrigger value="available">Available</TabsTrigger>
        </TabsList>

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

        {error && <p className="mb-4 text-sm text-muted-foreground">{error}</p>}

        <TabsContent value="installed">
          {loading ? (
            <LoadingSpinner text="Loading plugins..." className="py-8" />
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
                    onRemove={() => void handleRemove(plugin.id, plugin.name)}
                    updateAvailable={updateInfo}
                    updateStatus={updateStatus}
                    onUpdate={() => handleUpdate(plugin.name)}
                  />
                );
              })}
              {filtered.length === 0 && (
                <div className="py-8 text-center">
                  <p className="text-sm text-muted-foreground mb-2">No plugins installed.</p>
                  <Button variant="link" size="xs" onClick={() => setTab("available")}>
                    Browse available plugins
                  </Button>
                </div>
              )}
            </div>
          )}
        </TabsContent>

        <TabsContent value="available">
          {loadingAvailable ? (
            <LoadingSpinner text="Loading plugin registry..." className="py-8" />
          ) : (
            <div className="space-y-2">
              {available.map((plugin) => {
                const status = installStatus[plugin.name];
                return (
                  <ItemCard
                    key={plugin.name}
                    data-testid={`available-plugin-${plugin.name}`}
                    className="overflow-hidden p-0"
                  >
                    <div className="flex items-center gap-3 p-4">
                      <Package
                        size={20}
                        className={`shrink-0 ${plugin.installed || status?.state === "done" ? "text-primary" : "text-muted-foreground"}`}
                      />
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium">{plugin.name}</span>
                          <Badge variant="secondary">v{plugin.version}</Badge>
                          <Badge variant="secondary">{plugin.type}</Badge>
                        </div>
                        {plugin.description && (
                          <div className="text-xs text-muted-foreground mt-0.5">
                            {plugin.description}
                          </div>
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
                        <span className="text-[10px] text-muted-foreground px-2 py-0.5 rounded bg-muted">
                          Installed
                        </span>
                      ) : status?.state === "downloading" ? (
                        <div className="flex items-center gap-1 text-xs text-muted-foreground">
                          <Loader2 size={12} className="animate-spin" />
                          {status.percent !== undefined
                            ? `${status.percent}%`
                            : t("Downloading...")}
                        </div>
                      ) : status?.state === "error" ? (
                        <div className="flex items-center gap-1">
                          <span className="text-[10px] text-destructive">
                            {status.error || "Failed"}
                          </span>
                          <Button
                            variant="link"
                            size="xs"
                            onClick={() => handleInstall(plugin.name)}
                          >
                            Retry
                          </Button>
                        </div>
                      ) : !plugin.available ? (
                        <div className="flex items-center gap-2">
                          <span className="text-[10px] text-muted-foreground">
                            {t("No build for")} {plugin.platform}
                          </span>
                          <Button
                            size="sm"
                            disabled
                            data-testid={`install-${plugin.name}`}
                            title={t("This plugin has no build for your platform")}
                          >
                            <Download size={12} /> Install
                          </Button>
                        </div>
                      ) : (
                        <Button
                          size="sm"
                          data-testid={`install-${plugin.name}`}
                          onClick={() => handleInstall(plugin.name)}
                        >
                          <Download size={12} /> Install
                        </Button>
                      )}
                    </div>
                  </ItemCard>
                );
              })}
              {available.length === 0 && (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  {search ? t("No plugins match your search.") : t("No plugins available.")}
                </p>
              )}
            </div>
          )}
        </TabsContent>
      </Tabs>
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
    <ItemCard data-testid={`installed-plugin-${plugin.name}`} className="overflow-hidden p-0">
      <div className="flex items-center gap-3 p-4">
        <Package size={20} className="shrink-0 text-primary" />
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm font-semibold">{plugin.name}</span>
            {plugin.framework_version && (
              <Badge variant="secondary">{plugin.framework_version}</Badge>
            )}
            <Badge variant="secondary" className="text-[10px]">
              v{plugin.version}
            </Badge>
            <Badge variant="secondary" className="text-[10px]">
              {plugin.type}
            </Badge>
          </div>
          {plugin.description && (
            <div className="text-xs text-muted-foreground mt-0.5">{plugin.description}</div>
          )}
          {plugin.formats && plugin.formats.length > 0 && (
            <div className="flex flex-wrap gap-1 mt-1.5">
              {plugin.formats.slice(0, 8).map((f) => (
                <span
                  key={f}
                  className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground font-mono"
                >
                  {f}
                </span>
              ))}
              {plugin.formats.length > 8 && (
                <span className="text-[10px] text-muted-foreground">
                  +{plugin.formats.length - 8} more
                </span>
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
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() => setExpanded(!expanded)}
              title="Show capabilities"
            >
              {expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
            </Button>
          )}
          {/* Update button / progress */}
          {updateStatus?.state === "downloading" ? (
            <div className="flex items-center gap-1 text-xs text-primary">
              <Loader2 size={12} className="animate-spin" />
              {updateStatus.percent !== undefined ? `${updateStatus.percent}%` : t("Updating...")}
            </div>
          ) : updateStatus?.state === "done" ? (
            <span className="text-[10px] text-primary px-2 py-0.5 rounded bg-primary/10">
              Updated
            </span>
          ) : updateStatus?.state === "error" ? (
            <div className="flex items-center gap-1">
              <span className="text-[10px] text-destructive">{updateStatus.error || "Failed"}</span>
              {onUpdate && (
                <Button variant="link" size="xs" onClick={onUpdate}>
                  Retry
                </Button>
              )}
            </div>
          ) : updateAvailable && onUpdate ? (
            <Button
              variant="outline"
              size="xs"
              onClick={onUpdate}
              className="border-primary/30 bg-primary/5 text-primary hover:bg-primary/10"
              title={t("Update to v{version}", { version: updateAvailable.latest_version ?? "" })}
            >
              <ArrowUpCircle size={11} />
              {updateAvailable.latest_version ? `v${updateAvailable.latest_version}` : t("Update")}
            </Button>
          ) : null}
          {/* Remove */}
          {removing ? (
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Loader2 size={12} className="animate-spin" /> Removing...
            </div>
          ) : confirmRemove ? (
            <div className="flex items-center gap-1">
              <Button variant="destructive" size="xs" onClick={onRemove}>
                Remove
              </Button>
              <Button variant="ghost" size="xs" onClick={onCancelRemove}>
                Cancel
              </Button>
            </div>
          ) : (
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={onConfirmRemove}
              className="hover:bg-destructive/10 hover:text-destructive"
              title="Uninstall"
            >
              <Trash2 size={12} />
            </Button>
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
                  <span
                    key={c.name}
                    className="text-[10px] px-1.5 py-0.5 rounded border border-border text-foreground"
                  >
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
                  <span
                    key={c.name}
                    className="text-[10px] px-1.5 py-0.5 rounded border border-border text-foreground"
                  >
                    {c.display_name || c.name}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </ItemCard>
  );
}
