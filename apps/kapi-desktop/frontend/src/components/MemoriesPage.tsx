import { useState, useEffect, useCallback } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { Database, Plus, FolderOpen, X, Upload, Download, AlertTriangle } from "lucide-react";
import {
  Button,
  Card,
  CardContent,
  Label,
  Input,
  PageHeader,
  ChartContainer,
  type ChartConfig,
} from "@neokapi/ui-primitives";
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip } from "recharts";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { useTMAdapter } from "../hooks/useTMAdapter";
import { useLocales } from "../hooks/useLocales";
import { TMBrowser, ResourceCard, ImportProgress, type ResourceInfo } from "@neokapi/ui-primitives";

export interface MemoriesPageProps {
  /** Project tab ID — when set, shows the project-scoped TM. */
  tabID?: string;
  /** Pre-loaded resources for Storybook — skips api.listNamedTMs(). */
  resources?: ResourceInfo[];
  /** Force loading/skeleton state (for Storybook). */
  forceLoading?: boolean;
}

interface ActivityPoint {
  date: string;
  count: number;
}

const chartConfig: ChartConfig = {
  count: { label: "Entries", color: "var(--chart-1)" },
};

export function MemoriesPage({
  tabID,
  resources: propResources,
  forceLoading = false,
}: MemoriesPageProps = {}) {
  const [resources, setResources] = useState<ResourceInfo[]>(propResources ?? []);
  const [loading, setLoading] = useState(forceLoading || !propResources);
  const [handle, setHandle] = useState<string | null>(null);
  const [tmName, setTmName] = useState("");
  const [tmPath, setTmPath] = useState("");
  const [importing, setImporting] = useState(false);
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [newName, setNewName] = useState("");
  const [corruptPath, setCorruptPath] = useState<string | null>(null);
  const [corruptName, setCorruptName] = useState("");
  const [recovering, setRecovering] = useState(false);

  // Project TM state
  const [projectHandle, setProjectHandle] = useState<string | null>(null);
  const [projectStats, setProjectStats] = useState<{ count: number } | null>(null);
  const [activityStats, setActivityStats] = useState<ActivityPoint[]>([]);
  const { showError } = useError();
  const { locales } = useLocales();
  const activeHandle = projectHandle || handle;
  const adapter = useTMAdapter(activeHandle);

  // Load the project-scoped TM handle when opened from a project tab so the
  // project's own TM is auto-selected (and marked) rather than a blank picker.
  useEffect(() => {
    if (!tabID) return;
    api
      .getProjectHandles(tabID)
      .then((h) => {
        if (h?.tmHandle) setProjectHandle(h.tmHandle);
      })
      .catch(() => {});
  }, [tabID]);

  // Dashboard stats (count, activity) for whichever TM is open — project OR
  // named. Both use the same view, so a named TM shows the same activity chart.
  useEffect(() => {
    if (!activeHandle) {
      setProjectStats(null);
      setActivityStats([]);
      return;
    }
    void api.getTMStats(activeHandle).then((s) => {
      if (s) setProjectStats(s);
    });
    void api.getTMActivityStats(activeHandle).then((stats) => {
      if (stats) setActivityStats(stats);
    });
  }, [activeHandle]);

  const refreshResources = useCallback(async () => {
    if (propResources || forceLoading) return;
    setLoading(true);
    try {
      const list = await api.listNamedTMs();
      setResources(list ?? []);
    } catch (err) {
      showError("Failed to load translation memories", err);
    } finally {
      setLoading(false);
    }
  }, [showError, propResources, forceLoading]);

  useEffect(() => {
    if (!projectHandle) void refreshResources();
  }, [refreshResources, projectHandle]);

  const handleOpen = useCallback(async (path: string, name: string) => {
    try {
      const h = await api.openTM(path);
      if (h) {
        setHandle(h);
        setTmName(name);
        setTmPath(path);
      }
    } catch {
      setCorruptPath(path);
      setCorruptName(name);
    }
  }, []);

  const handleRecover = useCallback(async () => {
    if (!corruptPath) return;
    setRecovering(true);
    try {
      await api.recoverResource(corruptPath);
      const h = await api.createTM(corruptPath);
      if (h) {
        setHandle(h);
        setTmName(corruptName);
        setTmPath(corruptPath);
      }
      setCorruptPath(null);
      setCorruptName("");
    } catch (err) {
      showError("Recovery failed", err);
    } finally {
      setRecovering(false);
    }
  }, [corruptPath, corruptName, showError]);

  const handleOpenDialog = useCallback(async () => {
    try {
      const h = await api.openTMDialog();
      if (h) {
        setHandle(h);
        setTmName("Translation Memory");
        setTmPath("");
      }
    } catch (err) {
      showError("Failed to open translation memory", err);
    }
  }, [showError]);

  const handleCreate = useCallback(async () => {
    if (!newName.trim()) return;
    try {
      const h = await api.createNamedTM(newName.trim());
      if (h) {
        setHandle(h);
        setTmName(newName.trim());
        setTmPath("");
        setShowCreateDialog(false);
        setNewName("");
      }
    } catch (err) {
      showError("Failed to create translation memory", err);
    }
  }, [newName, showError]);

  const handleClose = useCallback(() => {
    if (handle) {
      void api.closeTM(handle);
      setHandle(null);
      setTmName("");
      setTmPath("");
      void refreshResources();
    }
  }, [handle, refreshResources]);

  const handleImport = useCallback(async () => {
    if (!activeHandle) return;
    setImporting(true);
    try {
      await api.importTMXDialog(activeHandle);
    } catch (err) {
      showError("Failed to import TMX", err);
    } finally {
      setImporting(false);
    }
  }, [activeHandle, showError]);

  const handleExport = useCallback(async () => {
    if (!activeHandle) return;
    try {
      await api.exportTMXDialog(activeHandle, []);
    } catch (err) {
      showError("Failed to export TMX", err);
    }
  }, [activeHandle, showError]);

  // Open TM view — identical dashboard (stats + activity chart + browser) whether
  // the TM is project-scoped or a named/ad-hoc one. Only the header differs.
  if (activeHandle && adapter) {
    const isProject = !!projectHandle;
    return (
      <div className="p-6">
        <PageHeader
          title={isProject ? "Project Translation Memory" : tmName}
          subtitle={
            projectStats ? `${projectStats.count.toLocaleString()} entries` : tmPath || undefined
          }
          backButton={
            isProject ? undefined : (
              <Button variant="ghost" size="icon-xs" onClick={handleClose} title="Close TM">
                <X size={16} />
              </Button>
            )
          }
          actions={
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={handleImport}>
                <Upload size={12} />
                Import TMX
              </Button>
              <Button variant="outline" size="sm" onClick={handleExport}>
                <Download size={12} />
                Export TMX
              </Button>
            </div>
          }
        />

        {/* Activity chart */}
        {activityStats.length > 0 && (
          <Card className="mb-6">
            <CardContent className="p-4">
              <div className="mb-2 text-sm font-medium">Activity</div>
              <ChartContainer config={chartConfig} className="aspect-auto h-40 w-full">
                <AreaChart data={activityStats} margin={{ left: 0, right: 0, top: 4, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} />
                  <XAxis
                    dataKey="date"
                    tickFormatter={(v: string) => {
                      const d = new Date(v);
                      return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
                    }}
                    className="text-[10px]"
                  />
                  <YAxis width={40} className="text-[10px]" />
                  <Tooltip
                    labelFormatter={(v) => new Date(String(v)).toLocaleDateString()}
                    formatter={(v) => [`${String(v)} entries`, "Entries"]}
                  />
                  <Area
                    type="monotone"
                    dataKey="count"
                    stroke="var(--color-count)"
                    fill="var(--color-count)"
                    fillOpacity={0.15}
                    strokeWidth={2}
                  />
                </AreaChart>
              </ChartContainer>
            </CardContent>
          </Card>
        )}

        <TMBrowser adapter={adapter} locales={locales} onError={showError} />
        <ImportProgress active={importing} />
      </div>
    );
  }

  // Resource picker view — no TM open.
  return (
    <div className="p-6">
      <PageHeader
        title="Translation Memories"
        actions={
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={handleOpenDialog}>
              <FolderOpen size={12} />
              Open File...
            </Button>
            <Button size="sm" onClick={() => setShowCreateDialog(true)}>
              <Plus size={12} />
              Create TM
            </Button>
          </div>
        }
      />

      {/* No project TM hint */}
      {tabID && !projectHandle && !loading && (
        <Card className="mb-4 border-dashed">
          <CardContent className="p-4 text-center text-sm text-muted-foreground">
            <Database size={16} className="mx-auto mb-1 opacity-50" />
            No project translation memory found. Run a translation flow to create one automatically,
            or create one below.
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
        {loading
          ? [0, 1, 2].map((i) => (
              <ResourceCard key={i} loading name="" path="" onClick={() => {}} />
            ))
          : resources.map((r) => (
              <ResourceCard
                key={r.path}
                name={r.name}
                path={r.path}
                size={r.size}
                modified={r.modified}
                icon={<Database size={18} />}
                onClick={() => void handleOpen(r.path, r.name)}
              />
            ))}
      </div>

      {/* Empty state */}
      {!loading && resources.length === 0 && !tabID && (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <Database size={24} className="mx-auto mb-2 text-muted-foreground/50" />
            <p className="mb-3 text-sm text-muted-foreground">
              No translation memories found. Create one or open a .db file.
            </p>
            <div className="flex justify-center gap-2">
              <Button size="sm" onClick={() => setShowCreateDialog(true)}>
                Create TM
              </Button>
              <Button variant="outline" size="sm" onClick={handleOpenDialog}>
                Open File...
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Create dialog */}
      {showCreateDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
            <h2 className="mb-3 text-lg font-semibold">New Translation Memory</h2>
            <Label className="mb-1 block text-xs text-muted-foreground">Name</Label>
            <Input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void handleCreate();
              }}
              placeholder="my-project"
              autoFocus
              className="mb-4"
            />
            <div className="flex gap-2">
              <Button size="sm" onClick={() => void handleCreate()} disabled={!newName.trim()}>
                Create
              </Button>
              <Button variant="outline" size="sm" onClick={() => setShowCreateDialog(false)}>
                Cancel
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Corruption recovery dialog */}
      {corruptPath && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
            <div className="mb-3 flex items-center gap-2">
              <AlertTriangle size={18} className="text-destructive" />
              <h2 className="text-base font-semibold">Corrupt Translation Memory</h2>
            </div>
            <p className="mb-2 text-sm text-muted-foreground">
              <strong>{corruptName}</strong> could not be opened. The database may be corrupt.
            </p>
            <p className="mb-4 text-xs text-muted-foreground">
              The file will be renamed to{" "}
              <code className="rounded bg-muted px-1 py-0.5 text-[10px]">.db.bak</code> and a fresh
              database created in its place.
            </p>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="destructive"
                onClick={() => void handleRecover()}
                disabled={recovering}
              >
                {recovering ? t("Recovering...") : t("Recover")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setCorruptPath(null);
                  setCorruptName("");
                }}
              >
                Cancel
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
