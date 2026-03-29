import { useState, useEffect, useCallback } from "react";
import { Plus, Trash2, Globe, FileText, FolderOpen, RefreshCw, Loader2 } from "lucide-react";
import type { KapiProject, ContentEntry } from "../types/api";
import { api } from "../hooks/useApi";

function shortenHome(path: string): string {
  const home = typeof process !== "undefined" ? process.env.HOME : undefined;
  if (home && path.startsWith(home)) {
    return "~" + path.slice(home.length);
  }
  // Client-side fallback: detect /Users/xxx or /home/xxx pattern.
  const match = path.match(/^(\/Users\/[^/]+|\/home\/[^/]+)(\/.*)?$/);
  if (match) {
    return "~" + (match[2] ?? "");
  }
  return path;
}

function pathDir(path: string): string {
  const i = path.lastIndexOf("/");
  return i > 0 ? path.slice(0, i) : path;
}

interface FileMatch {
  path: string;
  format: string;
  relative: string;
  pattern: string;
}

interface ContentPageProps {
  project: KapiProject;
  projectPath: string;
  onUpdate: (project: KapiProject) => void;
  tabID: string;
}

export function ContentPage({ project, projectPath, onUpdate, tabID }: ContentPageProps) {
  const [matches, setMatches] = useState<FileMatch[]>([]);
  const [basePath, setBasePath] = useState("");
  const [scanning, setScanning] = useState(false);

  const content = project.content ?? [];

  const rescanFiles = useCallback(async () => {
    setScanning(true);
    // Sync current project state to backend before scanning.
    await api.updateProject(tabID, project);
    const [files, base] = await Promise.all([
      api.matchContent(tabID),
      api.getBasePath(tabID),
    ]);
    setMatches(files ?? []);
    setBasePath(base ?? "");
    setScanning(false);
  }, [tabID, project]);

  useEffect(() => {
    rescanFiles();
  }, [rescanFiles, content.length]);

  const handleAddEntry = () => {
    onUpdate({
      ...project,
      content: [...content, { path: "" }],
    });
  };

  const handleUpdateEntry = (index: number, entry: ContentEntry) => {
    const updated = [...content];
    updated[index] = entry;
    onUpdate({ ...project, content: updated });
  };

  const handleDeleteEntry = (index: number) => {
    onUpdate({
      ...project,
      content: content.filter((_, i) => i !== index),
    });
  };

  const handleUpdateTargets = (value: string) => {
    const targets = value.split(",").map((s) => s.trim()).filter(Boolean);
    onUpdate({ ...project, target_languages: targets });
  };

  return (
    <div className="p-6">
      <h1 className="mb-6 text-xl font-semibold">Content</h1>

      {/* Languages */}
      <section className="mb-6">
        <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          <Globe size={14} />
          Languages
        </h2>
        <div className="grid max-w-lg grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs text-muted-foreground" htmlFor="source-lang">
              Source Language
            </label>
            <input
              id="source-lang"
              type="text"
              value={project.source_language ?? ""}
              onChange={(e) => onUpdate({ ...project, source_language: e.target.value })}
              placeholder="en-US"
              className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-muted-foreground" htmlFor="target-langs">
              Target Languages
            </label>
            <input
              id="target-langs"
              type="text"
              value={project.target_languages?.join(", ") ?? ""}
              onChange={(e) => handleUpdateTargets(e.target.value)}
              placeholder="fr-FR, de-DE, ja-JP"
              className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>
      </section>

      {/* Base path */}
      <section className="mb-6">
        <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          <FolderOpen size={14} />
          Base Path
        </h2>
        <div className="max-w-lg">
          <input
            type="text"
            value={project.base_path ?? ""}
            onChange={(e) => onUpdate({ ...project, base_path: e.target.value || undefined })}
            placeholder="Defaults to project file directory"
            className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            aria-label="Project base path"
          />
          <p className="mt-1 text-xs text-muted-foreground">
            {shortenHome(basePath || projectPath ? pathDir(projectPath) : "")}
          </p>
        </div>
      </section>

      {/* File patterns */}
      <section className="mb-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            <FileText size={14} />
            File Patterns
          </h2>
          <button
            onClick={handleAddEntry}
            className="flex items-center gap-1 rounded-md border border-border px-2.5 py-1 text-xs hover:bg-accent"
            aria-label="Add content pattern"
          >
            <Plus size={12} />
            Add Pattern
          </button>
        </div>

        {content.length > 0 ? (
          <div className="space-y-2">
            {content.map((entry, i) => (
              <div key={i} className="group rounded-lg border border-border p-3">
                <div className="grid grid-cols-3 gap-2">
                  <div>
                    <label className="mb-0.5 block text-xs text-muted-foreground">Path pattern</label>
                    <input
                      type="text"
                      value={entry.path}
                      onChange={(e) => handleUpdateEntry(i, { ...entry, path: e.target.value })}
                      placeholder="src/locales/en/*.json"
                      className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                    />
                  </div>
                  <div>
                    <label className="mb-0.5 block text-xs text-muted-foreground">Format</label>
                    <input
                      type="text"
                      value={entry.format ?? ""}
                      onChange={(e) => handleUpdateEntry(i, { ...entry, format: e.target.value || undefined })}
                      placeholder="auto-detect"
                      className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                    />
                  </div>
                  <div className="flex items-end gap-1">
                    <div className="flex-1">
                      <label className="mb-0.5 block text-xs text-muted-foreground">Target path</label>
                      <input
                        type="text"
                        value={entry.target ?? ""}
                        onChange={(e) => handleUpdateEntry(i, { ...entry, target: e.target.value || undefined })}
                        placeholder="src/locales/{lang}/*.json"
                        className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                      />
                    </div>
                    <button
                      onClick={() => handleDeleteEntry(i)}
                      className="rounded p-1 text-muted-foreground opacity-0 hover:text-destructive group-hover:opacity-100"
                      aria-label={`Remove pattern ${i + 1}`}
                    >
                      <Trash2 size={12} />
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="rounded-lg border border-dashed border-border p-6 text-center">
            <FileText size={20} className="mx-auto mb-2 text-muted-foreground/50" />
            <p className="text-sm text-muted-foreground">
              No content patterns. Add a pattern to map your source files.
            </p>
          </div>
        )}
      </section>

      {/* Matched files */}
      <section>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Matched Files ({matches.length})
          </h2>
          <button
            onClick={rescanFiles}
            disabled={scanning}
            className="flex items-center gap-1 rounded-md border border-border px-2.5 py-1 text-xs hover:bg-accent disabled:opacity-50"
            aria-label="Rescan files"
          >
            {scanning ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
            Rescan
          </button>
        </div>

        {matches.length > 0 ? (
          <div className="rounded-lg border border-border">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-border text-left text-muted-foreground">
                  <th className="px-3 py-2 font-medium">File</th>
                  <th className="px-3 py-2 font-medium">Format</th>
                  <th className="px-3 py-2 font-medium">Pattern</th>
                </tr>
              </thead>
              <tbody>
                {matches.map((m, i) => (
                  <tr key={i} className="border-b border-border last:border-0 hover:bg-accent/30">
                    <td className="px-3 py-1.5 font-mono">{m.relative}</td>
                    <td className="px-3 py-1.5">
                      <span className="rounded bg-accent px-1.5 py-0.5">{m.format || "unknown"}</span>
                    </td>
                    <td className="px-3 py-1.5 text-muted-foreground">{m.pattern}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : content.length > 0 ? (
          <p className="text-sm text-muted-foreground">
            {scanning ? "Scanning..." : "No files matched the configured patterns."}
          </p>
        ) : null}
      </section>
    </div>
  );
}
