import { useState } from "react";
import { Globe, FileText, Workflow, Save, Loader2 } from "lucide-react";
import type { KapiProject, TabInfo } from "../types/api";
import { api } from "../hooks/useApi";

interface ProjectPageProps {
  project: KapiProject;
  projectPath: string;
  onSaved?: (tab: TabInfo) => void;
  tabID: string;
}

export function ProjectPage({ project, projectPath, onSaved, tabID }: ProjectPageProps) {
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    setSaving(true);
    try {
      if (projectPath) {
        await api.saveProject(tabID);
      } else {
        const updated = await api.saveProjectDialog(tabID);
        if (updated) onSaved?.(updated);
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">{project.name}</h1>
          {projectPath ? (
            <p className="mt-1 text-sm text-muted-foreground">{projectPath}</p>
          ) : (
            <p className="mt-1 text-sm text-muted-foreground">Not yet saved to disk</p>
          )}
        </div>
        <button
          onClick={handleSave}
          disabled={saving}
          className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent disabled:opacity-50"
          aria-label={projectPath ? "Save project" : "Save project as"}
        >
          {saving ? <Loader2 size={12} className="animate-spin" /> : <Save size={12} />}
          {projectPath ? "Save" : "Save As..."}
        </button>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {/* Languages card */}
        <div className="rounded-lg border border-border p-4">
          <div className="mb-3 flex items-center gap-2">
            <Globe size={16} className="text-primary" />
            <h2 className="text-sm font-medium">Languages</h2>
          </div>
          <div className="space-y-1 text-sm">
            <div>
              <span className="text-muted-foreground">Source: </span>
              <span>{project.source_language || "Not set"}</span>
            </div>
            <div>
              <span className="text-muted-foreground">Targets: </span>
              <span>
                {project.target_languages?.length ? project.target_languages.join(", ") : "None"}
              </span>
            </div>
          </div>
        </div>

        {/* Content card */}
        <div className="rounded-lg border border-border p-4">
          <div className="mb-3 flex items-center gap-2">
            <FileText size={16} className="text-primary" />
            <h2 className="text-sm font-medium">Content</h2>
          </div>
          <div className="space-y-1 text-sm">
            {project.content?.length ? (
              project.content.map((entry, i) => (
                <div key={i} className="truncate text-muted-foreground">
                  {entry.path}
                  {entry.format && <span className="ml-1 text-xs">({entry.format})</span>}
                </div>
              ))
            ) : (
              <p className="text-muted-foreground">No content patterns</p>
            )}
          </div>
        </div>

        {/* Flows card */}
        <div className="rounded-lg border border-border p-4">
          <div className="mb-3 flex items-center gap-2">
            <Workflow size={16} className="text-primary" />
            <h2 className="text-sm font-medium">Flows</h2>
          </div>
          <div className="space-y-1 text-sm">
            {project.flows && Object.keys(project.flows).length > 0 ? (
              Object.entries(project.flows).map(([name, spec]) => (
                <div key={name} className="text-muted-foreground">
                  {name}
                  <span className="ml-1 text-xs">
                    ({spec.steps.length} step{spec.steps.length !== 1 ? "s" : ""})
                  </span>
                </div>
              ))
            ) : (
              <p className="text-muted-foreground">No flows defined</p>
            )}
          </div>
        </div>
      </div>

      {/* Preset & plugins */}
      {(project.preset || project.plugins?.length) && (
        <div className="mt-6 space-y-2 text-sm">
          {project.preset && (
            <div>
              <span className="text-muted-foreground">Preset: </span>
              <span className="rounded bg-accent px-1.5 py-0.5 text-xs">{project.preset}</span>
            </div>
          )}
          {project.plugins?.length ? (
            <div>
              <span className="text-muted-foreground">Plugins: </span>
              {project.plugins.map((p) => (
                <span key={p} className="mr-1 rounded bg-accent px-1.5 py-0.5 text-xs">
                  {p}
                </span>
              ))}
            </div>
          ) : null}
        </div>
      )}
    </div>
  );
}
