import { useState, useEffect, useCallback } from "react";
import { BookOpen, Plus, FolderOpen, X, Upload, AlertTriangle } from "lucide-react";
import { Button, Label, Input, PageHeader, EmptyState } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { useTermbaseAdapter } from "../hooks/useTermbaseAdapter";
import {
  TermbaseBrowser,
  ResourceCard,
  ImportProgress,
  type ResourceInfo,
} from "@neokapi/ui-primitives";

export function TermbasesPage() {
  const [resources, setResources] = useState<ResourceInfo[]>([]);
  const [loading, setLoading] = useState(true);
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
  const adapter = useTermbaseAdapter(handle);

  const refreshResources = useCallback(async () => {
    setLoading(true);
    try {
      const list = await api.listNamedTermbases();
      setResources(list ?? []);
    } catch (err) {
      showError("Failed to load termbases", err);
    } finally {
      setLoading(false);
    }
  }, [showError]);

  useEffect(() => {
    void refreshResources();
  }, [refreshResources]);

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
      api.closeTermbase(handle);
      setHandle(null);
      setTbName("");
      setTbPath("");
      void refreshResources();
    }
  }, [handle, refreshResources]);

  const handleImportCSV = useCallback(async () => {
    if (!handle) return;
    setImporting(true);
    try {
      await api.importTermbaseCSVDialog(handle, "", "", "");
    } catch (err) {
      showError("Failed to import CSV", err);
    } finally {
      setImporting(false);
    }
  }, [handle, showError]);

  const handleImportJSON = useCallback(async () => {
    if (!handle) return;
    setImporting(true);
    try {
      await api.importTermbaseJSONDialog(handle);
    } catch (err) {
      showError("Failed to import JSON", err);
    } finally {
      setImporting(false);
    }
  }, [handle, showError]);

  const handleExport = useCallback(async () => {
    if (!handle) return;
    try {
      await api.exportTermbaseJSONDialog(handle, tbName || "termbase");
    } catch (err) {
      showError("Failed to export termbase", err);
    }
  }, [handle, tbName, showError]);

  // Browser view — termbase is open.
  if (handle && adapter) {
    return (
      <div className="p-6">
        <PageHeader
          title={tbName}
          subtitle={tbPath || undefined}
          backButton={
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={handleClose}
              title="Close Termbase"
            >
              <X size={16} />
            </Button>
          }
          actions={
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={handleImportCSV}
              >
                <Upload size={12} />
                Import CSV
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleImportJSON}
              >
                <Upload size={12} />
                Import JSON
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleExport}
              >
                Export JSON
              </Button>
            </div>
          }
        />

        <TermbaseBrowser adapter={adapter} onError={showError} />

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
              New Termbase
            </Button>
          </div>
        }
      />

      <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
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

      {!loading && resources.length === 0 && (
        <EmptyState
          icon={<BookOpen size={24} className="text-muted-foreground/50" />}
          title="No termbases found. Create one or open a .db file."
          action={
            <div className="flex gap-2 justify-center">
              <Button
                size="sm"
                onClick={() => setShowCreateDialog(true)}
              >
                New Termbase
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleOpenDialog}
              >
                Open File...
              </Button>
            </div>
          }
        />
      )}

      {showCreateDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
            <h2 className="text-lg font-semibold mb-3">New Termbase</h2>
            <Label className="text-xs text-muted-foreground block mb-1">Name</Label>
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
              <h2 className="text-base font-semibold">Corrupt Termbase</h2>
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
                {recovering ? "Recovering..." : "Create Fresh Termbase"}
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
