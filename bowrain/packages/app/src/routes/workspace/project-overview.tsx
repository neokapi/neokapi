// The Overview: a Bowrain project's landing page, drawn by the project screens
// over the server's REST routes.

import { useEffect } from "react";
import { useParams, useRouteContext } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "@neokapi/ui";
import {
  OverviewScreen,
  type ProjectScreensPort,
  type ScreenTarget,
} from "@neokapi/project-screens";
import type { WorkspaceRouteContext } from "..";
import { projectQueryOptions } from "../../queries";
import { bowrainOpens, useProjectScreensPort } from "./project-screens-port";
import { useScreenNavigate } from "./project-screens-nav";

export interface ProjectOverviewPageProps {
  port: ProjectScreensPort;
  projectName: string;
  navigate: (target: ScreenTarget) => void;
}

/** The Overview of one project, read through the port it is given. */
export function ProjectOverviewPage({ port, projectName, navigate }: ProjectOverviewPageProps) {
  return (
    <OverviewScreen
      port={port}
      projectName={projectName}
      navigate={navigate}
      canNavigate={bowrainOpens}
    />
  );
}

export function ProjectOverviewRoute() {
  const { projectId = "", stream = "main" } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const api = useApi();
  const scope = { ws, projectId, stream };
  const port = useProjectScreensPort(scope);
  const navigate = useScreenNavigate(scope);
  const { data: project } = useQuery(projectQueryOptions(api, ws, projectId, stream));
  const projectName = project?.name ?? projectId;

  useEffect(() => {
    document.title = `${projectName} · Bowrain`;
  }, [projectName]);

  return <ProjectOverviewPage port={port} projectName={projectName} navigate={navigate} />;
}
