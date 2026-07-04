import { useEffect, useMemo } from "react";
import { useRouteContext } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";
import { projectsQueryOptions } from "../../queries";
import { LocaleDemandPage } from "../../locale-demand/LocaleDemandPage";

/**
 * Locale demand — prototype surface in `src/locale-demand/`; this route wires
 * the workspace context around it. Demand comes from the PostHog connector
 * when one is configured, otherwise the sample dataset. The connector is
 * project-scoped; the workspace-level page uses the first project (phase 0 —
 * a project picker comes with the plan-join work).
 */
export function LocaleDemandRoute() {
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const api = useApi();

  useEffect(() => {
    document.title = `Locale demand — ${activeWorkspace.name} — Bowrain`;
  }, [activeWorkspace.name]);

  const { data: projects, isPending } = useQuery(projectsQueryOptions(api, activeWorkspace.slug));
  const project = projects?.[0];

  const projectLocales = useMemo(() => {
    if (!project) return [];
    return [project.default_source_language, ...project.target_languages];
  }, [project]);

  // The page decides live-vs-sample once per mount, so wait for the project
  // list (usually already in the router loader cache) and remount on change.
  if (isPending) return null;

  return (
    <LocaleDemandPage
      key={project?.id ?? "no-project"}
      api={api}
      workspaceSlug={activeWorkspace.slug}
      projectId={project?.id}
      projectLocales={projectLocales}
    />
  );
}
