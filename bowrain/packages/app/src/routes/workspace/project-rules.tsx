// Rules: the questions about a Bowrain project's suggestions, answered by a
// member, and the rules in force with who kept each.

import { useEffect } from "react";
import { useParams, useRouteContext, useSearch } from "@tanstack/react-router";
import {
  DecideScreen,
  KeptRulesScreen,
  type ProjectScreensPort,
  type ScreenTarget,
} from "@neokapi/project-screens";
import type { WorkspaceRouteContext } from "..";
import { bowrainOpens, useProjectScreensPort } from "./project-screens-port";
import { useScreenNavigate } from "./project-screens-nav";

export interface ProjectRulesPageProps {
  port: ProjectScreensPort;
  /** The Map row to open on (`suggested:<id>`, `conflict:<id>`). */
  row?: string;
  navigate: (target: ScreenTarget) => void;
}

/** Decide and the kept rules of one project, read through the port it is given. */
export function ProjectRulesPage({ port, row, navigate }: ProjectRulesPageProps) {
  return (
    <div data-slot="project-rules" className="flex min-h-full flex-col">
      <div className="h-[36rem] shrink-0 border-b border-border">
        <DecideScreen
          key={row}
          port={port}
          defaultSelected={row}
          navigate={navigate}
          canNavigate={bowrainOpens}
        />
      </div>
      <div className="mx-auto w-full max-w-4xl p-6">
        <KeptRulesScreen port={port} />
      </div>
    </div>
  );
}

export function ProjectRulesRoute() {
  const { projectId = "", stream = "main" } = useParams({ strict: false });
  const { row } = useSearch({ strict: false }) as { row?: string };
  const { activeWorkspace, user } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const scope = { ws: activeWorkspace.slug, projectId, stream };
  const port = useProjectScreensPort(scope, user);
  const navigate = useScreenNavigate(scope);

  useEffect(() => {
    document.title = `Rules · Bowrain`;
  }, []);

  return <ProjectRulesPage port={port} row={row} navigate={navigate} />;
}
