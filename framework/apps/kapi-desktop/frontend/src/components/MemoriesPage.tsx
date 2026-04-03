import { useState, useEffect, useCallback } from "react";
import { Database, Plus, FolderOpen, X, Upload, Download, AlertTriangle } from "lucide-react";
import { Button, Card, CardContent, Label, Input, PageHeader, SkeletonCard } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { useTMAdapter } from "../hooks/useTMAdapter";
import { TMBrowser, ResourceCard, ImportProgress, type ResourceInfo } from "@neokapi/ui-primitives";

export function MemoriesPage() {
  const [resources, setResources] = useState<ResourceInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [handle, setHandle] = useState<string | null>(null);
  const [tmName, setTmName] = useState("");
  const [tmPath, setTmPath] = useState("");
  const [importing, setImporting] = useState(false);
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [newName, setNewName] = useState("");
  const [corruptPath, setCorruptPath] = useState<string | null>(null);
  const [corruptName, setCorruptName] = useState("");
  const [recovering, setRecovering] = useState(false);

  const { showError } = useError();
  const adapter = useTMAdapter(handle);

  const refreshResources = useCallback(async () => {
    setLoading(true);
    try {
      const list = await api.listNamedTMs();
      setResources(list ?? []);
    } catch (err) {
      showError("Failed to load translation memories", err);
    } finally {
      setLoading(false);
    }
  }, [showError]);

  useEffect(() => {
    void refreshResources();
  }, [refreshResources]);

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
      api.closeTM(handle);
      setHandle(null);
      setTmName("");
      setTmPath("");
      void refreshResources();
    }
  }, [handle, refreshResources]);

  const handleImport = useCallback(async () => {
    if (!handle) return;
    setImporting(true);
    try {
      await api.importTMXDialog(handle, "", "");
    } catch (err) {
      showError("Failed to import TMX", err);
    } finally {
      setImporting(false);
    }
  }, [handle, showError]);

  const handleExport = useCallback(async () => {
    if (!handle) return;
    try {
      await api.exportTMXDialog(handle, "", "");
    } catch (err) {
      showError("Failed to export TMX", err);
    }
  }, [handle, showError]);

  // Browser view — TM is open.
  if (handle && adapter) {
    return (
      <div className="p-6">
        <PageHeader
          title={tmName}
          subtitle={tmPath || undefined}
          backButton={
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={handleClose}
              title="Close TM"
            >
              <X size={16} />
            </Button>
          }
          actions={
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={handleImport}
              >
                <Upload size={12} />
                Import TMX
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleExport}
              >
                <Download size={12} />
                Export TMX
              </Button>
            </div>
          }
        />

        <TMBrowser adapter={adapter} showLookup onError={showError} />

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
            <Button
              variant="outline"
              size="sm"
              onClick={handleOpenDialog}
            >
              <FolderOpen size={12} />
              Open File...
            </Button>
            <Button
              size="sm"
              onClick={() => setShowCreateDialog(true)}
            >
              <Plus size={12} />
              Create TM
            </Button>
          </div>
        }
      />

      {/* Loading skeleton */}
      {loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {[0, 1, 2].map((i) => (
            <SkeletonCard key={i} lines={2} />
          ))}
        </div>
      )}

      {/* Named TMs list */}
      {!loading && resources.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {resources.map((r) => (
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
      )}

      {/* Empty state */}
      {!loading && resources.length === 0 && (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <Database size={24} className="mx-auto mb-2 text-muted-foreground/50" />
            <p className="text-sm text-muted-foreground mb-3">
              No translation memories found. Create one or open a .db file.
            </p>
            <div className="flex gap-2 justify-center">
              <Button
                size="sm"
                onClick={() => setShowCreateDialog(true)}
              >
                Create TM
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleOpenDialog}
              >
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
            <h2 className="text-lg font-semibold mb-3">New Translation Memory</h2>
            <Label className="text-xs text-muted-foreground block mb-1">Name</Label>
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
              <Button
                size="sm"
                onClick={() => void handleCreate()}
                disabled={!newName.trim()}
              >
                Create
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setShowCreateDialog(false)}
              >
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
            <div className="flex items-center gap-2 mb-3">
              <AlertTriangle size={18} className="text-destructive" />
              <h2 className="text-base font-semibold">Corrupt Translation Memory</h2>
            </div>
            <p className="text-sm text-muted-foreground mb-2">
              <strong>{corruptName}</strong> could not be opened. The database may be corrupt.
            </p>
            <p className="text-xs text-muted-foreground mb-4">
              The file will be renamed to <code className="text-[10px] bg-muted px-1 py-0.5 rounded">.db.bak</code> and a fresh database created in its place.
            </p>
            <div className="flex gap-2">
              <Button
                size="sm"
                onClick={() => void handleRecover()}
                disabled={recovering}
              >
                {recovering ? "Recovering..." : "Create Fresh TM"}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => { setCorruptPath(null); setCorruptName(""); }}
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
