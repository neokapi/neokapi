// A project's content memory, or a named one: its pairs as a list beside a
// detail pane (MemoryListDetail). With no store open, the page lists the named
// stores to open one from.

import { useState, useEffect, useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import { Database, Plus, FolderOpen, X, AlertTriangle } from "lucide-react";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  EmptyState,
  Label,
  Input,
  PageHeader,
  SimpleTooltip,
  type MemoryAdapter,
  type ResourceInfo,
} from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useError } from "./ErrorBanner";
import { useActiveFilter } from "../context/ActiveFilterContext";
import { useMemoryAdapter } from "../hooks/useMemoryAdapter";
import { useLocales } from "../hooks/useLocales";
import { MemoryListDetail } from "./MemoryListDetail";
import { StoreList } from "./StoreList";

export interface MemoriesPageProps {
  /** Project tab ID — when set, shows the project-scoped Memory. */
  tabID?: string;
  /** Pre-loaded resources for Storybook — skips api.listNamedMemories(). */
  resources?: ResourceInfo[];
  /** Force loading/skeleton state (for Storybook). */
  forceLoading?: boolean;
  /**
   * Open the context surface standing at an entry's unit. Absent where there is
   * none to open — the ad-hoc store has no project to stand in.
   */
  onOpenUnit?: (unitPath: string) => void;
  /**
   * A store already open, for Storybook: skips the handle lookup and reads the
   * entries through this adapter.
   */
  store?: { name: string; adapter: MemoryAdapter; count?: number; defaultSelected?: string };
}

export function MemoriesPage({
  tabID,
  resources: propResources,
  forceLoading = false,
  onOpenUnit,
  store,
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
  const [adding, setAdding] = useState(false);

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
  const activeHandle = projectHandle || handle;
  const hostAdapter = useMemoryAdapter(activeHandle, tabID, onOpenUnit);
  const adapter = store?.adapter ?? hostAdapter;

  // The count in the header, for whichever store is open: the project's or a
  // named one. Both open on the same list and detail.
  const statsQuery = useQuery({
    queryKey: qk.memoryStats(activeHandle ?? ""),
    queryFn: () => api.getMemoryStats(activeHandle!),
    enabled: !!activeHandle,
  });
  const projectStats = activeHandle ? (statsQuery.data ?? null) : null;

  // Named-content memory list — only when there is no project content memory to show instead.
  const resourcesQuery = useQuery({
    queryKey: qk.namedMemories(),
    queryFn: () => api.listNamedMemories(),
    enabled: !propResources && !forceLoading && !projectHandle && !store,
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

  // An open store, project-scoped or named: the header, then the pairs as a
  // list beside a detail pane filling the rest of the page.
  if (adapter && (activeHandle || store)) {
    const isProject = !!projectHandle || !!store;
    const count = store?.count ?? projectStats?.count;
    return (
      <div className="flex h-full min-h-0 flex-col">
        <PageHeader
          className="mb-0 shrink-0 px-6 pt-5 pb-3"
          title={store?.name ?? (isProject ? "Project Content Memory" : memoryName)}
          subtitle={
            count !== undefined
              ? t("{count, plural, one {# entry} other {# entries}}", { count })
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
          actions={
            <Button size="sm" data-slot="memory-add" onClick={() => setAdding(true)}>
              <Plus size={12} />
              Add entry
            </Button>
          }
        />
        <div className="min-h-0 flex-1 border-t border-border">
          <MemoryListDetail
            adapter={adapter}
            handle={activeHandle ?? `story:${store?.name ?? ""}`}
            targetLocales={filterLangs.length ? filterLangs : undefined}
            locales={locales}
            sourceLocale={sourceLang}
            adding={adding}
            onAddingChange={setAdding}
            defaultSelected={store?.defaultSelected}
            onError={showError}
          />
        </div>
      </div>
    );
  }

  // No store open: the named stores, one row each, to open one from.
  return (
    <div className="mx-auto max-w-3xl p-6">
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

      {tabID && !projectHandle && !loading && (
        <EmptyState
          className="mb-4"
          icon={<Database size={20} />}
          title="This project has no content memory"
          description="One is created the first time a translation flow runs. You can also create one here, or open one you already have."
        />
      )}

      <StoreList
        stores={resources}
        loading={loading}
        icon={<Database size={16} />}
        onOpen={(r) => void handleOpen(r.path, r.name)}
        aria-label={t("Content memories")}
      />

      {!loading && resources.length === 0 && !tabID && (
        <EmptyState
          icon={<Database size={24} className="text-muted-foreground/50" />}
          title="No content memories found. Create one or open a .db file."
          action={
            <div className="flex justify-center gap-2">
              <Button size="sm" onClick={() => setShowCreateDialog(true)}>
                New memory
              </Button>
              <Button variant="outline" size="sm" onClick={handleOpenDialog}>
                Open File...
              </Button>
            </div>
          }
        />
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
