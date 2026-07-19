import { useState, useEffect, useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import { BookOpen, Plus, FolderOpen, X, Upload, AlertTriangle } from "lucide-react";
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
  EmptyState,
  ChartContainer,
  SimpleTooltip,
  type ChartConfig,
} from "@neokapi/ui-primitives";
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip } from "recharts";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useError } from "./ErrorBanner";
import { useActiveFilter } from "../context/ActiveFilterContext";
import { ConceptsView } from "./ConceptsView";
import { ResourceCard, ImportProgress, type ResourceInfo } from "@neokapi/ui-primitives";

export interface TermbasesPageProps {
  /** Project tab ID — when set, shows the project-scoped termbase. */
  tabID?: string;
  /** Pre-loaded resources for Storybook — skips api.listNamedTermbases(). */
  resources?: ResourceInfo[];
  /** Force loading/skeleton state (for Storybook). */
  forceLoading?: boolean;
}

interface ActivityPoint {
  date: string;
  count: number;
}

const chartConfig: ChartConfig = {
  count: { label: "Concepts", color: "var(--chart-2)" },
};

export function TermbasesPage({
  tabID,
  resources: propResources,
  forceLoading = false,
}: TermbasesPageProps = {}) {
  const qc = useQueryClient();
  const [handle, setHandle] = useState<string | null>(null);
  const [tbName, setTbName] = useState("");
  const [tbPath, setTbPath] = useState("");
  const [importing, setImporting] = useState(false);
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [newName, setNewName] = useState("");
  const [corruptPath, setCorruptPath] = useState<string | null>(null);
  const [corruptName, setCorruptName] = useState("");
  const [recovering, setRecovering] = useState(false);

  const { showError } = useError();
  const { active: activeFilter } = useActiveFilter();

  // Project-scoped termbase handle + source locale (auto-selects the project's
  // own termbase rather than a blank picker). Read-only server state.
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
  const projectHandle = tabID ? (projectHandlesQuery.data?.termbaseHandle ?? null) : null;
  const sourceLang = projectQuery.data?.defaults?.source_language ?? "";
  const activeHandle = projectHandle || handle;

  // Scope concept terms to the Active Filter's languages plus the project source
  // locale (so a concept's canonical name stays visible). No filter → show all.
  const localeScope = activeFilter?.languages?.length
    ? [sourceLang, ...activeFilter.languages].filter(Boolean)
    : undefined;

  // Dashboard stats (count, activity, locales) for whichever termbase is open —
  // project OR named. Both use the same view, so a named termbase shows the same
  // activity chart + filters as a project one.
  const statsQuery = useQuery({
    queryKey: qk.termbaseStats(activeHandle ?? ""),
    queryFn: () => api.getTermbaseStats(activeHandle!),
    enabled: !!activeHandle,
  });
  const activityQuery = useQuery({
    queryKey: qk.termbaseActivityStats(activeHandle ?? ""),
    queryFn: () => api.getTermbaseActivityStats(activeHandle!),
    enabled: !!activeHandle,
  });
  const projectStats = activeHandle ? (statsQuery.data ?? null) : null;
  const activityStats: ActivityPoint[] = activeHandle ? (activityQuery.data ?? []) : [];

  // Named-termbase list — only when there is no project termbase to show instead.
  const resourcesQuery = useQuery({
    queryKey: qk.namedTermbases(),
    queryFn: () => api.listNamedTermbases(),
    enabled: !propResources && !forceLoading && !projectHandle,
  });
  const resources: ResourceInfo[] = propResources ?? resourcesQuery.data ?? [];
  const loading = forceLoading || (!propResources && !projectHandle && resourcesQuery.isLoading);

  useEffect(() => {
    if (resourcesQuery.error) {
      showError("Failed to load termbases", resourcesQuery.error);
    }
  }, [resourcesQuery.error, showError]);

  const refreshResources = useCallback(() => {
    void qc.invalidateQueries({ queryKey: qk.namedTermbases() });
  }, [qc]);

  const handleOpen = useCallback(async (path: string, name: string) => {
    try {
      const h = await api.openTermbase(path);
      if (h) {
        setHandle(h);
        setTbName(name);
        setTbPath(path);
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
      const h = await api.createTermbase(corruptPath);
      if (h) {
        setHandle(h);
        setTbName(corruptName);
        setTbPath(corruptPath);
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
      const h = await api.openTermbaseDialog();
      if (h) {
        setHandle(h);
        setTbName("Termbase");
        setTbPath("");
      }
    } catch (err) {
      showError("Failed to open termbase", err);
    }
  }, [showError]);

  const handleCreate = useCallback(async () => {
    if (!newName.trim()) return;
    try {
      const h = await api.createNamedTermbase(newName.trim());
      if (h) {
        setHandle(h);
        setTbName(newName.trim());
        setTbPath("");
        setShowCreateDialog(false);
        setNewName("");
      }
    } catch (err) {
      showError("Failed to create termbase", err);
    }
  }, [newName, showError]);

  const handleClose = useCallback(() => {
    if (handle) {
      void api.closeTermbase(handle);
      setHandle(null);
      setTbName("");
      setTbPath("");
      refreshResources();
    }
  }, [handle, refreshResources]);

  const handleImportCSV = useCallback(async () => {
    if (!activeHandle) return;
    setImporting(true);
    try {
      await api.importTermbaseCSVDialog(activeHandle, "", "", "");
    } catch (err) {
      showError("Failed to import CSV", err);
    } finally {
      setImporting(false);
    }
  }, [activeHandle, showError]);

  const handleImportJSON = useCallback(async () => {
    if (!activeHandle) return;
    setImporting(true);
    try {
      await api.importTermbaseJSONDialog(activeHandle);
    } catch (err) {
      showError("Failed to import JSON", err);
    } finally {
      setImporting(false);
    }
  }, [activeHandle, showError]);

  const handleExport = useCallback(async () => {
    if (!activeHandle) return;
    try {
      await api.exportTermbaseJSONDialog(activeHandle, tbName || "termbase");
    } catch (err) {
      showError("Failed to export termbase", err);
    }
  }, [activeHandle, tbName, showError]);

  // Open termbase view — identical dashboard (stats + activity chart + the
  // visual concept/relation workspace) whether the termbase is project-scoped or
  // a named/ad-hoc one. Only the header (title, back button) differs.
  if (activeHandle) {
    const isProject = !!projectHandle;
    return (
      <div className="p-6">
        <PageHeader
          title={isProject ? "Project Termbase" : tbName}
          subtitle={
            projectStats ? `${projectStats.count.toLocaleString()} concepts` : tbPath || undefined
          }
          backButton={
            isProject ? undefined : (
              <SimpleTooltip content="Close Termbase">
                <Button
                  variant="ghost"
                  size="icon-xs"
                  onClick={handleClose}
                  aria-label="Close Termbase"
                >
                  <X size={16} />
                </Button>
              </SimpleTooltip>
            )
          }
          actions={
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={handleImportCSV}>
                <Upload size={12} />
                Import CSV
              </Button>
              <Button variant="outline" size="sm" onClick={handleImportJSON}>
                <Upload size={12} />
                Import JSON
              </Button>
              <Button variant="outline" size="sm" onClick={handleExport}>
                Export JSON
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
                    formatter={(v) => [`${String(v)} concepts`, "Concepts"]}
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

        {/* Visual concept/relation workspace (browse → open → relate / re-status) */}
        <ConceptsView handle={activeHandle} localeScope={localeScope} />
        <ImportProgress active={importing} />
      </div>
    );
  }

  // Resource picker view.
  return (
    <div className="p-6">
      <PageHeader
        title="Termbases"
        actions={
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={handleOpenDialog}>
              <FolderOpen size={12} />
              Open File...
            </Button>
            <Button size="sm" onClick={() => setShowCreateDialog(true)}>
              <Plus size={12} />
              New Termbase
            </Button>
          </div>
        }
      />

      {/* No project termbase hint */}
      {tabID && !projectHandle && !loading && (
        <Card className="mb-4 border-dashed">
          <CardContent className="p-4 text-center text-sm text-muted-foreground">
            <BookOpen size={16} className="mx-auto mb-1 opacity-50" />
            No project termbase found. Import terminology to create one automatically, or create one
            below.
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
                icon={<BookOpen size={18} />}
                onClick={() => void handleOpen(r.path, r.name)}
              />
            ))}
      </div>

      {!loading && resources.length === 0 && !tabID && (
        <EmptyState
          icon={<BookOpen size={24} className="text-muted-foreground/50" />}
          title="No termbases found. Create one or open a .db file."
          action={
            <div className="flex justify-center gap-2">
              <Button size="sm" onClick={() => setShowCreateDialog(true)}>
                New Termbase
              </Button>
              <Button variant="outline" size="sm" onClick={handleOpenDialog}>
                Open File...
              </Button>
            </div>
          }
        />
      )}

      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>New Termbase</DialogTitle>
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
              placeholder="my-glossary"
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
              Corrupt Termbase
            </DialogTitle>
          </DialogHeader>
          <div>
            <p className="mb-2 text-sm text-muted-foreground">
              <strong>{corruptName}</strong> could not be opened.
            </p>
            <p className="mb-4 text-xs text-muted-foreground">
              The file will be renamed to{" "}
              <code className="rounded bg-muted px-1 py-0.5 text-[10px]">.db.bak</code> and a fresh
              database created.
            </p>
            <div className="flex gap-2">
              <Button size="sm" onClick={() => void handleRecover()} disabled={recovering}>
                {recovering ? t("Recovering...") : t("Create Fresh Termbase")}
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
