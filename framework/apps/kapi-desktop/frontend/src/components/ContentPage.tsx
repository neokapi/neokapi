import { useState, useEffect, useCallback } from "react";
import { Plus, Trash2, Globe, FileText, FolderOpen, RefreshCw, Loader2, X } from "lucide-react";
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
  const [presets, setPresets] = useState<Array<{ name: string; description: string }>>([]);
  const [formats, setFormats] = useState<string[]>([]);

  const content = project.content ?? [];

  // Load available presets and formats on mount.
  useEffect(() => {
    api.listPresets().then((p) => { if (p) setPresets(p); });
    api.listFormats().then((f) => { if (f) setFormats(f.map((x) => x.name)); });
  }, []);

  // Fetch base path on mount and when project changes.
  useEffect(() => {
    api.updateProject(tabID, project).then(() => {
      api.getBasePath(tabID).then((base) => {
        if (base) setBasePath(base);
      });
    });
  }, [tabID, project.base_path, projectPath]);

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

  return (
    <div className="p-6">
      <h1 className="mb-6 text-xl font-semibold">Content</h1>

      {/* Framework preset */}
      {presets.length > 0 && (
        <section className="mb-6">
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Preset
          </h2>
          <div className="flex max-w-lg items-center gap-2">
            <select
              value={project.preset ?? ""}
              onChange={async (e) => {
                const name = e.target.value;
                if (name) {
                  const updated = await api.applyPreset(tabID, name);
                  if (updated) onUpdate(updated);
                } else {
                  onUpdate({ ...project, preset: undefined });
                }
              }}
              className="flex-1 rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
              aria-label="Framework preset"
            >
              <option value="">None (custom)</option>
              {presets.map((p) => (
                <option key={p.name} value={p.name}>
                  {p.name} — {p.description}
                </option>
              ))}
            </select>
            {project.preset && (
              <span className="rounded bg-accent px-2 py-0.5 text-xs">{project.preset}</span>
            )}
          </div>
        </section>
      )}

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
            <label className="mb-1 block text-xs text-muted-foreground">
              Target Languages
            </label>
            <div className="flex flex-wrap items-center gap-1.5 rounded border border-input bg-transparent px-2 py-1.5">
              {(project.target_languages ?? []).map((lang) => (
                <span
                  key={lang}
                  className="flex items-center gap-1 rounded bg-accent px-2 py-0.5 text-xs"
                >
                  {lang}
                  <button
                    onClick={() =>
                      onUpdate({
                        ...project,
                        target_languages: project.target_languages?.filter((l) => l !== lang),
                      })
                    }
                    className="ml-0.5 rounded-full p-0.5 text-muted-foreground hover:text-destructive"
                    aria-label={`Remove ${lang}`}
                  >
                    <X size={10} />
                  </button>
                </span>
              ))}
              <input
                type="text"
                placeholder={project.target_languages?.length ? "" : "Add language (e.g. fr-FR)"}
                className="min-w-[80px] flex-1 bg-transparent text-sm outline-none"
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === ",") {
                    e.preventDefault();
                    const val = e.currentTarget.value.trim();
                    if (val && !project.target_languages?.includes(val)) {
                      onUpdate({
                        ...project,
                        target_languages: [...(project.target_languages ?? []), val],
                      });
                      e.currentTarget.value = "";
                    }
                  }
                }}
              />
            </div>
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
            placeholder={shortenHome(basePath) || "Set project base path"}
            className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            aria-label="Project base path"
          />
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
                    <select
                      value={entry.format ?? ""}
                      onChange={(e) => handleUpdateEntry(i, { ...entry, format: e.target.value || undefined })}
                      className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                    >
                      <option value="">auto-detect</option>
                      {formats.map((f) => (
                        <option key={f} value={f}>{f}</option>
                      ))}
                    </select>
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
