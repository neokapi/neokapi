import { useState, useCallback } from "react";
import { Globe, FileText, Workflow, Save, Loader2, Pencil } from "lucide-react";
import { Button, Badge, Card, CardHeader, CardTitle, CardContent } from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { KapiProject, TabInfo } from "../types/api";
import { isBareEntry, effectiveItems } from "../types/api";
import { api } from "../hooks/useApi";
import { ConvergencePanel } from "./ConvergencePanel";

interface ProjectPageProps {
  project: KapiProject;
  projectPath: string;
  onSaved?: (tab: TabInfo) => void;
  onProjectChange?: (project: KapiProject) => void;
  tabID: string;
}

/** Derive the display name: explicit name from YAML, or folder name from path. */
function displayName(project: KapiProject, projectPath: string): string {
  if (project.name) return project.name;
  if (!projectPath) return "Untitled";
  // Path is like /Users/.../MyApp/project.kapi — grab "MyApp"
  const parts = projectPath.replace(/\/project\.kapi$/i, "").split("/");
  return parts[parts.length - 1] || "Untitled";
}

export function ProjectPage({
  project,
  projectPath,
  onSaved,
  onProjectChange,
  tabID,
}: ProjectPageProps) {
  const [saving, setSaving] = useState(false);
  const [editingName, setEditingName] = useState(false);
  const [nameInput, setNameInput] = useState("");

  const defaults = project.defaults ?? {};
  const plugins = project.plugins ?? {};

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

  const handleStartEditName = useCallback(() => {
    setNameInput(project.name || "");
    setEditingName(true);
  }, [project.name]);

  const handleSaveName = useCallback(async () => {
    const trimmed = nameInput.trim();
    const updated = { ...project, name: trimmed };
    await api.updateProject(tabID, updated);
    onProjectChange?.(updated);
    if (projectPath) await api.saveProject(tabID);
    setEditingName(false);
  }, [nameInput, project, tabID, projectPath, onProjectChange]);

  const handleCancelEditName = useCallback(() => {
    setEditingName(false);
  }, []);

  const name = displayName(project, projectPath);

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          {editingName ? (
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={nameInput}
                onChange={(e) => setNameInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void handleSaveName();
                  if (e.key === "Escape") handleCancelEditName();
                }}
                placeholder={displayName({ ...project, name: "" }, projectPath)}
                autoFocus
                className="rounded-md border border-input bg-transparent px-2 py-1 text-xl font-semibold outline-none focus:ring-2 focus:ring-ring"
              />
              <Button variant="outline" size="xs" onClick={handleSaveName}>
                Save
              </Button>
              <Button variant="outline" size="xs" onClick={handleCancelEditName}>
                Cancel
              </Button>
            </div>
          ) : (
            <div className="group flex items-center gap-2">
              <h1 className="text-xl font-semibold">{name}</h1>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={handleStartEditName}
                className="opacity-0 group-hover:opacity-100"
                aria-label="Edit project name"
              >
                <Pencil size={14} />
              </Button>
            </div>
          )}
          {projectPath ? (
            <p className="mt-1 text-sm text-muted-foreground">{projectPath}</p>
          ) : (
            <p className="mt-1 text-sm text-muted-foreground">Not yet saved to disk</p>
          )}
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={handleSave}
          disabled={saving}
          aria-label={projectPath ? "Save project" : "Save project as"}
        >
          {saving ? <Loader2 size={12} className="animate-spin" /> : <Save size={12} />}
          {projectPath ? t("Save") : t("Save As...")}
        </Button>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {/* Languages card */}
        <Card>
          <CardHeader className="px-4">
            <CardTitle className="flex items-center gap-2 text-sm font-medium">
              <Globe size={16} className="text-primary" />
              Languages
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4">
            <div className="space-y-1 text-sm">
              <div>
                <span className="text-muted-foreground">Source: </span>
                <span>{defaults.source_language || "Not set"}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Targets: </span>
                <span>
                  {defaults.target_languages?.length
                    ? defaults.target_languages.join(", ")
                    : t("None")}
                </span>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Content card */}
        <Card>
          <CardHeader className="px-4">
            <CardTitle className="flex items-center gap-2 text-sm font-medium">
              <FileText size={16} className="text-primary" />
              Content
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4">
            <div className="space-y-1 text-sm">
              {project.content?.length ? (
                project.content.map((coll, i) => {
                  if (isBareEntry(coll)) {
                    return (
                      <div key={i} className="truncate text-muted-foreground">
                        {coll.path}
                        {coll.format && <span className="ml-1 text-xs">({coll.format.name})</span>}
                      </div>
                    );
                  }
                  return (
                    <div key={i} className="text-muted-foreground">
                      {coll.name || t("Unnamed")}
                      <span className="ml-1 text-xs">
                        {t("({count} item(s))", { count: effectiveItems(coll).length })}
                      </span>
                    </div>
                  );
                })
              ) : (
                <p className="text-muted-foreground">No content patterns</p>
              )}
            </div>
          </CardContent>
        </Card>

        {/* Flows card */}
        <Card>
          <CardHeader className="px-4">
            <CardTitle className="flex items-center gap-2 text-sm font-medium">
              <Workflow size={16} className="text-primary" />
              Flows
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4">
            <div className="space-y-1 text-sm">
              {project.flows && Object.keys(project.flows).length > 0 ? (
                Object.entries(project.flows).map(([name, spec]) => (
                  <div key={name} className="text-muted-foreground">
                    {name}
                    <span className="ml-1 text-xs">
                      {t("({count} step(s))", { count: spec.steps.length })}
                    </span>
                  </div>
                ))
              ) : (
                <p className="text-muted-foreground">No flows defined</p>
              )}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Convergence — derived coverage, ship gates, and the review queue. */}
      <div className="mt-6">
        <ConvergencePanel tabID={tabID} />
      </div>

      {/* Preset & plugins */}
      {(project.preset || Object.keys(plugins).length > 0) && (
        <div className="mt-6 space-y-2 text-sm">
          {project.preset && (
            <div>
              <span className="text-muted-foreground">Preset: </span>
              <Badge variant="secondary">{project.preset}</Badge>
            </div>
          )}
          {Object.keys(plugins).length > 0 && (
            <div>
              <span className="text-muted-foreground">Plugins: </span>
              {Object.entries(plugins).map(([name, spec]) => (
                <Badge key={name} variant="secondary" className="mr-1" translate="no">
                  {name}
                  {spec.version && spec.version !== "*" ? ` ${spec.version}` : ""}
                </Badge>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
