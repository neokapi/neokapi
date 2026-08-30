import { useState, useEffect, useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import { Database, Plus, FolderOpen, X, AlertTriangle } from "lucide-react";
import {
  Button,
  Card,
  CardContent,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Label,
  Input,
  PageHeader,
  ChartContainer,
  SimpleTooltip,
  type ChartConfig,
} from "@neokapi/ui-primitives";
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip } from "recharts";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useError } from "./ErrorBanner";
import { useActiveFilter } from "../context/ActiveFilterContext";
import { useMemoryAdapter } from "../hooks/useMemoryAdapter";
import { useLocales } from "../hooks/useLocales";
import { MemoryBrowser, ResourceCard, type ResourceInfo } from "@neokapi/ui-primitives";

export interface MemoriesPageProps {
  /** Project tab ID — when set, shows the project-scoped Memory. */
  tabID?: string;
  /** Pre-loaded resources for Storybook — skips api.listNamedMemories(). */
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
  const qc = useQueryClient();
  const [handle, setHandle] = useState<string | null>(null);
  const [memoryName, setTmName] = useState("");
  const [memoryPath, setTmPath] = useState("");
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [newName, setNewName] = useState("");
  const [corruptPath, setCorruptPath] = useState<string | null>(null);
  const [corruptName, setCorruptName] = useState("");
  const [recovering, setRecovering] = useState(false);

  const { showError } = useError();
  const { locales } = useLocales();
  const { active: activeFilter } = useActiveFilter();

  // Project-scoped content-memory handle + source locale (auto-selects the project's own content memory
  // rather than a blank picker). Read-only server state via react-query.
  const projectHandlesQuery = useQuery({
    queryKey: qk.projectHandles(tabID ?? ""),
    queryFn: () => api.getProjectHandles(tabID!),
    enabled: !!tabID,
  });
  const projectQuery = useQuery({
    queryKey: qk.project(tabID ?? ""),
    queryFn: () => api.getProject(tabID!),
    enabled: !!tabID,
  });
  const projectHandle = tabID ? (projectHandlesQuery.data?.memoryHandle ?? null) : null;
  const sourceLang = projectQuery.data?.defaults?.source_language ?? "";

  // Focus the content memory on the Active Filter's languages: the multi-language view shows
  // the source plus the chosen targets, while the bilingual target defaults to a
  // chosen target. No filter → show all.
  const filterLangs = activeFilter?.languages ?? [];
  const scopeLocales = filterLangs.length
    ? [sourceLang, ...filterLangs].filter(Boolean)
    : undefined;
  const activeHandle = projectHandle || handle;
  const adapter = useMemoryAdapter(activeHandle);

  // Dashboard stats (count, activity) for whichever content memory is open — project OR
  // named. Both use the same view, so a named content memory shows the same activity chart.
  const statsQuery = useQuery({
    queryKey: qk.memoryStats(activeHandle ?? ""),
    queryFn: () => api.getMemoryStats(activeHandle!),
    enabled: !!activeHandle,
  });
  const activityQuery = useQuery({
    queryKey: qk.memoryActivityStats(activeHandle ?? ""),
    queryFn: () => api.getMemoryActivityStats(activeHandle!),
    enabled: !!activeHandle,
  });
  const projectStats = activeHandle ? (statsQuery.data ?? null) : null;
  const activityStats: ActivityPoint[] = activeHandle ? (activityQuery.data ?? []) : [];

  // Named-content memory list — only when there is no project content memory to show instead.
  const resourcesQuery = useQuery({
    queryKey: qk.namedMemories(),
    queryFn: () => api.listNamedMemories(),
    enabled: !propResources && !forceLoading && !projectHandle,
  });
  const resources: ResourceInfo[] = propResources ?? resourcesQuery.data ?? [];
  const loading = forceLoading || (!propResources && !projectHandle && resourcesQuery.isLoading);

  useEffect(() => {
    if (resourcesQuery.error) {
      showError("Failed to load content memories", resourcesQuery.error);
    }
  }, [resourcesQuery.error, showError]);

  const refreshResources = useCallback(() => {
    void qc.invalidateQueries({ queryKey: qk.namedMemories() });
  }, [qc]);

  const handleOpen = useCallback(async (path: string, name: string) => {
    try {
      const h = await api.openMemory(path);
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
      const h = await api.createMemory(corruptPath);
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
      const h = await api.openMemoryDialog();
      if (h) {
        setHandle(h);
        setTmName("Content Memory");
        setTmPath("");
      }
    } catch (err) {
      showError("Failed to open content memory", err);
    }
  }, [showError]);

  const handleCreate = useCallback(async () => {
    if (!newName.trim()) return;
    try {
      const h = await api.createNamedMemory(newName.trim());
      if (h) {
        setHandle(h);
        setTmName(newName.trim());
        setTmPath("");
        setShowCreateDialog(false);
        setNewName("");
      }
    } catch (err) {
      showError("Failed to create content memory", err);
    }
  }, [newName, showError]);

  const handleClose = useCallback(() => {
    if (handle) {
      void api.closeMemory(handle);
      setHandle(null);
      setTmName("");
      setTmPath("");
      refreshResources();
    }
  }, [handle, refreshResources]);

  // Open content memory view — identical dashboard (stats + activity chart + browser) whether
  // the content memory is project-scoped or a named/ad-hoc one. Only the header differs.
  if (activeHandle && adapter) {
    const isProject = !!projectHandle;
    return (
      <div className="p-6">
        <PageHeader
          title={isProject ? "Project Content Memory" : memoryName}
          subtitle={
            projectStats
              ? `${projectStats.count.toLocaleString()} entries`
              : memoryPath || undefined
          }
          backButton={
            isProject ? undefined : (
              <SimpleTooltip content="Close memory">
                <Button
                  variant="ghost"
                  size="icon-xs"
                  onClick={handleClose}
                  aria-label="Close memory"
                >
                  <X size={16} />
                </Button>
              </SimpleTooltip>
            )
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

        <MemoryBrowser
          adapter={adapter}
          locales={locales}
          scopeLocales={scopeLocales}
          targetLocales={filterLangs.length ? filterLangs : undefined}
          onError={showError}
        />
      </div>
    );
  }

  // Resource picker view — no content memory open.
  return (
    <div className="p-6">
      <PageHeader
        title="Content Memories"
        actions={
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={handleOpenDialog}>
              <FolderOpen size={12} />
              Open File...
            </Button>
            <Button size="sm" onClick={() => setShowCreateDialog(true)}>
              <Plus size={12} />
              New memory
            </Button>
          </div>
        }
      />

      {/* No project content memory hint */}
      {tabID && !projectHandle && !loading && (
        <Card className="mb-4 border-dashed">
          <CardContent className="p-4 text-center text-sm text-muted-foreground">
            <Database size={16} className="mx-auto mb-1 opacity-50" />
            No project content memory found. Run a translation flow to create one automatically, or
            create one below.
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
              No content memories found. Create one or open a .db file.
            </p>
            <div className="flex justify-center gap-2">
              <Button size="sm" onClick={() => setShowCreateDialog(true)}>
                New memory
              </Button>
              <Button variant="outline" size="sm" onClick={handleOpenDialog}>
                Open File...
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Create dialog */}
      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>New Content Memory</DialogTitle>
          </DialogHeader>
          <div>
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
        </DialogContent>
      </Dialog>

      {/* Corruption recovery dialog */}
      <Dialog
        open={!!corruptPath}
        onOpenChange={(o) => {
          if (!o) {
            setCorruptPath(null);
            setCorruptName("");
          }
        }}
      >
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle size={18} className="text-destructive" />
              Corrupt Content Memory
            </DialogTitle>
          </DialogHeader>
          <div>
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
        </DialogContent>
      </Dialog>
    </div>
  );
}
