// A project's terms, or a named terms store: the concepts as a list beside a
// detail pane (ConceptsView). With no store open, the page lists the named
// stores to open one from.

import { useState, useEffect, useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import { BookOpen, Plus, FolderOpen, X, AlertTriangle } from "lucide-react";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Label,
  Input,
  PageHeader,
  EmptyState,
  SimpleTooltip,
  type ResourceInfo,
} from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useError } from "./ErrorBanner";
import { useActiveFilter } from "../context/ActiveFilterContext";
import type { ConceptDataSource } from "@neokapi/concept-ui";
import { ConceptsView } from "./ConceptsView";
import { StoreList } from "./StoreList";

export interface TermsPageProps {
  /** Project tab ID — when set, shows the project-scoped terms. */
  tabID?: string;
  /** Pre-loaded resources for Storybook — skips api.listNamedTerms(). */
  resources?: ResourceInfo[];
  /** Force loading/skeleton state (for Storybook). */
  forceLoading?: boolean;
  /**
   * A store already open, for Storybook: skips the handle lookup and reads the
   * concepts from this source.
   */
  store?: { name: string; source: ConceptDataSource; count?: number; defaultSelected?: string };
}

export function TermsPage({
  tabID,
  resources: propResources,
  forceLoading = false,
  store,
}: TermsPageProps = {}) {
  const qc = useQueryClient();
  const [handle, setHandle] = useState<string | null>(null);
  const [tbName, setTbName] = useState("");
  const [tbPath, setTbPath] = useState("");
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [newName, setNewName] = useState("");
  const [corruptPath, setCorruptPath] = useState<string | null>(null);
  const [corruptName, setCorruptName] = useState("");
  const [recovering, setRecovering] = useState(false);

  const { showError } = useError();
  const { active: activeFilter } = useActiveFilter();

  // Project-scoped terms handle + source locale (auto-selects the project's
  // own terms rather than a blank picker). Read-only server state.
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
  const projectHandle = tabID ? (projectHandlesQuery.data?.termsHandle ?? null) : null;
  const sourceLang = projectQuery.data?.defaults?.source_language ?? "";
  const activeHandle = projectHandle || handle;

  // Scope concept terms to the Active Filter's languages plus the project source
  // locale (so a concept's canonical name stays visible). No filter → show all.
  const localeScope = activeFilter?.languages?.length
    ? [sourceLang, ...activeFilter.languages].filter(Boolean)
    : undefined;

  // The count in the header, for whichever store is open: the project's or a
  // named one. Both open on the same list and detail.
  const statsQuery = useQuery({
    queryKey: qk.termsStats(activeHandle ?? ""),
    queryFn: () => api.getTermsStats(activeHandle!),
    enabled: !!activeHandle,
  });
  const projectStats = activeHandle ? (statsQuery.data ?? null) : null;

  // Named-terms list — only when there is no project terms store to show instead.
  const resourcesQuery = useQuery({
    queryKey: qk.namedTerms(),
    queryFn: () => api.listNamedTerms(),
    enabled: !propResources && !forceLoading && !projectHandle && !store,
  });
  const resources: ResourceInfo[] = propResources ?? resourcesQuery.data ?? [];
  const loading = forceLoading || (!propResources && !projectHandle && resourcesQuery.isLoading);

  useEffect(() => {
    if (resourcesQuery.error) {
      showError("Failed to load terms", resourcesQuery.error);
    }
  }, [resourcesQuery.error, showError]);

  const refreshResources = useCallback(() => {
    void qc.invalidateQueries({ queryKey: qk.namedTerms() });
  }, [qc]);

  const handleOpen = useCallback(async (path: string, name: string) => {
    try {
      const h = await api.openTerms(path);
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
      const h = await api.createTerms(corruptPath);
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
      const h = await api.openTermsDialog();
      if (h) {
        setHandle(h);
        setTbName("Terms");
        setTbPath("");
      }
    } catch (err) {
      showError("Failed to open terms", err);
    }
  }, [showError]);

  const handleCreate = useCallback(async () => {
    if (!newName.trim()) return;
    try {
      const h = await api.createNamedTerms(newName.trim());
      if (h) {
        setHandle(h);
        setTbName(newName.trim());
        setTbPath("");
        setShowCreateDialog(false);
        setNewName("");
      }
    } catch (err) {
      showError("Failed to create terms", err);
    }
  }, [newName, showError]);

  const handleClose = useCallback(() => {
    if (handle) {
      void api.closeTerms(handle);
      setHandle(null);
      setTbName("");
      setTbPath("");
      refreshResources();
    }
  }, [handle, refreshResources]);

  // An open store, project-scoped or named: the header, then the concepts as a
  // list beside a detail pane filling the rest of the page.
  if (activeHandle || store) {
    const isProject = !!projectHandle || !!store;
    const count = store?.count ?? projectStats?.count;
    return (
      <div className="flex h-full min-h-0 flex-col">
        <PageHeader
          className="mb-0 shrink-0 px-6 pt-5 pb-3"
          title={store?.name ?? (isProject ? "Project Terms" : tbName)}
          subtitle={
            count !== undefined
              ? t("{count, plural, one {# concept} other {# concepts}}", { count })
              : tbPath || undefined
          }
          backButton={
            isProject ? undefined : (
              <SimpleTooltip content="Close terms">
                <Button
                  variant="ghost"
                  size="icon-xs"
                  onClick={handleClose}
                  aria-label="Close terms"
                >
                  <X size={16} />
                </Button>
              </SimpleTooltip>
            )
          }
        />
        <div className="min-h-0 flex-1 border-t border-border">
          <ConceptsView
            handle={activeHandle ?? `story:${store?.name ?? ""}`}
            source={store?.source}
            defaultSelected={store?.defaultSelected}
            localeScope={localeScope}
            sourceLocale={sourceLang || (store ? "en-US" : "")}
          />
        </div>
      </div>
    );
  }

  // No store open: the named stores, one row each, to open one from.
  return (
    <div className="mx-auto max-w-3xl p-6">
      <PageHeader
        title="Terms"
        actions={
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={handleOpenDialog}>
              <FolderOpen size={12} />
              Open File...
            </Button>
            <Button size="sm" onClick={() => setShowCreateDialog(true)}>
              <Plus size={12} />
              New Terms
            </Button>
          </div>
        }
      />

      {tabID && !projectHandle && !loading && (
        <EmptyState
          className="mb-4"
          icon={<BookOpen size={20} />}
          title="This project has no terms store"
          description="Create one, or open a terms store you already have."
        />
      )}

      <StoreList
        stores={resources}
        loading={loading}
        icon={<BookOpen size={16} />}
        onOpen={(r) => void handleOpen(r.path, r.name)}
        aria-label={t("Terms stores")}
      />

      {!loading && resources.length === 0 && !tabID && (
        <EmptyState
          icon={<BookOpen size={24} className="text-muted-foreground/50" />}
          title="No terms found. Create one or open a .db file."
          action={
            <div className="flex justify-center gap-2">
              <Button size="sm" onClick={() => setShowCreateDialog(true)}>
                New Terms
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
            <DialogTitle>New Terms</DialogTitle>
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
              placeholder="my-terms"
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
              Corrupt Terms
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
                {recovering ? t("Recovering...") : t("Create Fresh Terms")}
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
