import { useState, useEffect, useCallback } from "react";
import { Database, Plus, FolderOpen, X, Upload, Download, AlertTriangle } from "lucide-react";
import { api } from "../hooks/useApi";
import { useTMAdapter } from "../hooks/useTMAdapter";
import {
  TMBrowser,
  ResourceCard,
  ImportProgress,
  type ResourceInfo,
} from "@neokapi/ui-primitives";

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

  const adapter = useTMAdapter(handle);

  const refreshResources = useCallback(async () => {
    setLoading(true);
    const list = await api.listNamedTMs();
    setResources(list ?? []);
    setLoading(false);
  }, []);

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
    } catch {
      // Recovery itself failed — nothing more we can do.
    } finally {
      setRecovering(false);
    }
  }, [corruptPath, corruptName]);

  const handleOpenDialog = useCallback(async () => {
    const h = await api.openTMDialog();
    if (h) {
      setHandle(h);
      setTmName("Translation Memory");
      setTmPath("");
    }
  }, []);

  const handleCreate = useCallback(async () => {
    if (!newName.trim()) return;
    const h = await api.createNamedTM(newName.trim());
    if (h) {
      setHandle(h);
      setTmName(newName.trim());
      setTmPath("");
      setShowCreateDialog(false);
      setNewName("");
    }
  }, [newName]);

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
    } finally {
      setImporting(false);
    }
  }, [handle]);

  const handleExport = useCallback(async () => {
    if (!handle) return;
    await api.exportTMXDialog(handle, "", "");
  }, [handle]);

  // Browser view — TM is open.
  if (handle && adapter) {
    return (
      <div className="p-6">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-3">
            <button
              onClick={handleClose}
              className="p-1 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
              title="Close TM"
            >
              <X size={16} />
            </button>
            <div>
              <h1 className="text-lg font-semibold">{tmName}</h1>
              {tmPath && (
                <p className="text-[11px] text-muted-foreground">{tmPath}</p>
              )}
            </div>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleImport}
              className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
            >
              <Upload size={12} />
              Import TMX
            </button>
            <button
              onClick={handleExport}
              className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
            >
              <Download size={12} />
              Export TMX
            </button>
          </div>
        </div>

        <TMBrowser adapter={adapter} showLookup />

        <ImportProgress active={importing} />
      </div>
    );
  }

  // Resource picker view — no TM open.
  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Translation Memories</h1>
        <div className="flex gap-2">
          <button
            onClick={handleOpenDialog}
            className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
          >
            <FolderOpen size={12} />
            Open File...
          </button>
          <button
            onClick={() => setShowCreateDialog(true)}
            className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          >
            <Plus size={12} />
            Create TM
          </button>
        </div>
      </div>

      {/* Loading skeleton */}
      {loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {[0, 1, 2].map((i) => (
            <div key={i} className="rounded-lg border border-border p-4 animate-pulse">
              <div className="h-3.5 bg-muted rounded w-1/3 mb-2" />
              <div className="h-2.5 bg-muted rounded w-2/3" />
            </div>
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
        <div className="rounded-lg border border-dashed border-border p-8 text-center">
          <Database size={24} className="mx-auto mb-2 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground mb-3">
            No translation memories found. Create one or open a .db file.
          </p>
          <div className="flex gap-2 justify-center">
            <button
              onClick={() => setShowCreateDialog(true)}
              className="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
            >
              Create TM
            </button>
            <button
              onClick={handleOpenDialog}
              className="rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent transition-colors"
            >
              Open File...
            </button>
          </div>
        </div>
      )}

      {/* Create dialog */}
      {showCreateDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
            <h2 className="text-lg font-semibold mb-3">New Translation Memory</h2>
            <label className="text-xs text-muted-foreground block mb-1">Name</label>
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void handleCreate();
              }}
              placeholder="my-project"
              autoFocus
              className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring mb-4"
            />
            <div className="flex gap-2">
              <button
                onClick={() => void handleCreate()}
                disabled={!newName.trim()}
                className="rounded-md bg-primary px-4 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                Create
              </button>
              <button
                onClick={() => setShowCreateDialog(false)}
                className="rounded-md border border-border px-4 py-2 text-xs hover:bg-accent transition-colors"
              >
                Cancel
              </button>
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
              <button
                onClick={() => void handleRecover()}
                disabled={recovering}
                className="rounded-md bg-primary px-4 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                {recovering ? "Recovering..." : "Create Fresh TM"}
              </button>
              <button
                onClick={() => { setCorruptPath(null); setCorruptName(""); }}
                className="rounded-md border border-border px-4 py-2 text-xs hover:bg-accent transition-colors"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
