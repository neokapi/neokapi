import { useState, useEffect, useCallback, DragEvent } from "react";
import { FileText, FolderOpen, Plus, RefreshCw, Loader2, Upload } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { useShortenHome } from "../hooks/useShortenHome";

interface ProjectFile {
  path: string;
  relative: string;
  format: string;
  size: number;
  is_dir: boolean;
}

interface ProjectFilesPageProps {
  tabID: string;
  basePath: string;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function ProjectFilesPage({ tabID, basePath }: ProjectFilesPageProps) {
  const shortenHome = useShortenHome();
  const [files, setFiles] = useState<ProjectFile[]>([]);
  const [loading, setLoading] = useState(false);
  const [dragging, setDragging] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const result = await api.listProjectFiles(tabID);
      setFiles(result ?? []);
    } finally {
      setLoading(false);
    }
  }, [tabID]);

  // Initial load.
  useEffect(() => {
    refresh();
  }, [refresh]);

  // Auto-refresh when files change on disk.
  useWailsEvent("project-files-changed", (data) => {
    if (data === tabID) refresh();
  });

  const handleAddFiles = async () => {
    const added = await api.addFilesDialog(tabID, "");
    if (added && added.length > 0) refresh();
  };

  const handleDrop = useCallback(
    async (e: DragEvent) => {
      e.preventDefault();
      setDragging(false);
      const items = e.dataTransfer?.files;
      if (!items || items.length === 0) return;
      // Wails maps dropped files to the native path via dataTransfer.
      // In a Wails webview, file.path gives us the native path.
      for (let i = 0; i < items.length; i++) {
        const file = items[i];
        const path = (file as unknown as { path?: string }).path;
        if (path) {
          await api.copyFileToProject(tabID, path, "");
        }
      }
      refresh();
    },
    [tabID, refresh],
  );

  const handleDragOver = useCallback((e: DragEvent) => {
    e.preventDefault();
    setDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: DragEvent) => {
    e.preventDefault();
    setDragging(false);
  }, []);

  const directories = files.filter((f) => f.is_dir);
  const regularFiles = files.filter((f) => !f.is_dir);

  return (
    <div className="p-6">
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Project Files
          </h2>
          {basePath && (
            <p className="mt-0.5 text-xs text-muted-foreground">{shortenHome(basePath)}</p>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddFiles}
            aria-label="Add files"
          >
            <Plus size={12} />
            Add Files
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            onClick={refresh}
            disabled={loading}
            aria-label="Refresh files"
          >
            {loading ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
          </Button>
        </div>
      </div>

      {/* Drop zone + file list */}
      <div
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        className={`min-h-[200px] rounded-lg border-2 transition-colors ${
          dragging
            ? "border-primary bg-primary/5"
            : files.length > 0
              ? "border-transparent"
              : "border-dashed border-border"
        }`}
      >
        {files.length > 0 ? (
          <div className="rounded-lg border border-border">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-border text-left text-muted-foreground">
                  <th className="px-3 py-2 font-medium">Name</th>
                  <th className="px-3 py-2 font-medium">Format</th>
                  <th className="px-3 py-2 text-right font-medium">Size</th>
                </tr>
              </thead>
              <tbody>
                {directories.map((f) => (
                  <tr
                    key={f.relative}
                    className="border-b border-border last:border-0 hover:bg-accent/30"
                  >
                    <td className="px-3 py-1.5">
                      <span className="flex items-center gap-1.5 font-mono">
                        <FolderOpen size={12} className="text-muted-foreground" />
                        {f.relative}/
                      </span>
                    </td>
                    <td className="px-3 py-1.5 text-muted-foreground">&mdash;</td>
                    <td className="px-3 py-1.5 text-right text-muted-foreground">&mdash;</td>
                  </tr>
                ))}
                {regularFiles.map((f) => (
                  <tr
                    key={f.relative}
                    className="border-b border-border last:border-0 hover:bg-accent/30"
                  >
                    <td className="px-3 py-1.5">
                      <span className="flex items-center gap-1.5 font-mono">
                        <FileText size={12} className="text-muted-foreground" />
                        {f.relative}
                      </span>
                    </td>
                    <td className="px-3 py-1.5">
                      {f.format ? (
                        <span className="rounded bg-accent px-1.5 py-0.5">{f.format}</span>
                      ) : (
                        <span className="text-muted-foreground">&mdash;</span>
                      )}
                    </td>
                    <td className="px-3 py-1.5 text-right text-muted-foreground">
                      {formatSize(f.size)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <Upload size={24} className="mb-3 text-muted-foreground/50" />
            <p className="text-sm text-muted-foreground">
              Drop files here or click <strong>Add Files</strong> to add them to the project.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
