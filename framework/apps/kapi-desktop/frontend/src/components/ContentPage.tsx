import { useState } from "react";
import { Plus, Trash2, Globe, FileText } from "lucide-react";
import type { KapiProject, ContentEntry } from "../types/api";

interface ContentPageProps {
  project: KapiProject;
  projectPath: string;
  onUpdate: (project: KapiProject) => void;
  tabID: string;
}

export function ContentPage({ project, onUpdate }: ContentPageProps) {
  const [editingIndex, setEditingIndex] = useState<number | null>(null);

  const content = project.content ?? [];

  const handleAddEntry = () => {
    const entry: ContentEntry = { path: "" };
    onUpdate({
      ...project,
      content: [...content, entry],
    });
    setEditingIndex(content.length);
  };

  const handleUpdateEntry = (index: number, entry: ContentEntry) => {
    const updated = [...content];
    updated[index] = entry;
    onUpdate({ ...project, content: updated });
  };

  const handleDeleteEntry = (index: number) => {
    const updated = content.filter((_, i) => i !== index);
    onUpdate({ ...project, content: updated });
    setEditingIndex(null);
  };

  const handleUpdateLanguage = (field: "source_language", value: string) => {
    onUpdate({ ...project, [field]: value });
  };

  const handleUpdateTargets = (value: string) => {
    const targets = value
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    onUpdate({ ...project, target_languages: targets });
  };

  return (
    <div className="p-6">
      <h1 className="mb-6 text-xl font-semibold">Content</h1>

      {/* Languages */}
      <section className="mb-8">
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
              onChange={(e) => handleUpdateLanguage("source_language", e.target.value)}
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

      {/* File patterns */}
      <section>
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
              <div
                key={i}
                className="group rounded-lg border border-border p-3"
              >
                <div className="grid grid-cols-3 gap-2">
                  <div>
                    <label className="mb-0.5 block text-xs text-muted-foreground">
                      Path pattern
                    </label>
                    <input
                      type="text"
                      value={entry.path}
                      onChange={(e) =>
                        handleUpdateEntry(i, { ...entry, path: e.target.value })
                      }
                      onFocus={() => setEditingIndex(i)}
                      placeholder="src/locales/en/*.json"
                      className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                    />
                  </div>
                  <div>
                    <label className="mb-0.5 block text-xs text-muted-foreground">
                      Format
                    </label>
                    <input
                      type="text"
                      value={entry.format ?? ""}
                      onChange={(e) =>
                        handleUpdateEntry(i, { ...entry, format: e.target.value || undefined })
                      }
                      placeholder="auto-detect"
                      className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                    />
                  </div>
                  <div className="flex items-end gap-1">
                    <div className="flex-1">
                      <label className="mb-0.5 block text-xs text-muted-foreground">
                        Target path
                      </label>
                      <input
                        type="text"
                        value={entry.target ?? ""}
                        onChange={(e) =>
                          handleUpdateEntry(i, { ...entry, target: e.target.value || undefined })
                        }
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
          <div className="rounded-lg border border-dashed border-border p-8 text-center">
            <FileText size={24} className="mx-auto mb-2 text-muted-foreground/50" />
            <p className="text-sm text-muted-foreground">
              No content patterns configured. Add a pattern to map your source files.
            </p>
          </div>
        )}
      </section>
    </div>
  );
}
