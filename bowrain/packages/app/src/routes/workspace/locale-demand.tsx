import { useEffect, useMemo, useState } from "react";
import { useRouteContext } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useApi, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";
import { projectsQueryOptions } from "../../queries";
import { LocaleDemandPage } from "../../locale-demand/LocaleDemandPage";

/**
 * Locale demand — surface in `src/locale-demand/`; this route wires the
 * workspace context around it. Demand comes from the PostHog connector when one
 * is configured, otherwise the sample dataset. The connector is project-scoped;
 * pick which project to scope to (defaults to the first).
 */
export function LocaleDemandRoute() {
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const api = useApi();

  useEffect(() => {
    document.title = `Locale demand · ${activeWorkspace.name} · Bowrain`;
  }, [activeWorkspace.name]);

  const { data: projects, isPending } = useQuery(projectsQueryOptions(api, activeWorkspace.slug));
  const [selectedProjectId, setSelectedProjectId] = useState<string | undefined>(undefined);
  const project = useMemo(
    () => projects?.find((p) => p.id === selectedProjectId) ?? projects?.[0],
    [projects, selectedProjectId],
  );

  const projectLocales = useMemo(() => {
    if (!project) return [];
    return [project.default_source_language, ...project.target_languages];
  }, [project]);

  // The page decides live-vs-sample once per mount, so wait for the project
  // list (usually already in the router loader cache) and remount on change.
  if (isPending) return null;

  return (
    <div>
      {projects && projects.length > 1 && (
        <div className="mx-auto w-full max-w-7xl px-6 pt-6">
          <label className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">Project</span>
            <Select value={project?.id ?? ""} onValueChange={setSelectedProjectId}>
              <SelectTrigger className="h-8 w-56">
                <SelectValue placeholder="Select a project" />
              </SelectTrigger>
              <SelectContent>
                {projects.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </label>
        </div>
      )}
      <LocaleDemandPage
        key={project?.id ?? "no-project"}
        api={api}
        workspaceSlug={activeWorkspace.slug}
        projectId={project?.id}
        projectLocales={projectLocales}
      />
    </div>
  );
}
