// The Reader: one item of a Bowrain project with its findings and the rules
// at it. A Fix files a source proposal; the proposal's id opens the list of
// open proposals, where a person with the permission approves it.

import { useEffect, useState } from "react";
import { useParams, useRouteContext, useSearch } from "@tanstack/react-router";
import { SourceProposalsDialog } from "@neokapi/ui";
import { ReaderScreen, type ProjectScreensPort, type ScreenTarget } from "@neokapi/project-screens";
import { t } from "@neokapi/i18n-react/runtime";
import type { WorkspaceRouteContext } from "..";
import { bowrainOpens, useProjectScreensPort } from "./project-screens-port";
import { useScreenNavigate } from "./project-screens-nav";

export interface ProjectReaderPageProps {
  port: ProjectScreensPort;
  projectId: string;
  /** The item, as the route's splat names it. */
  path: string;
  block?: string;
  navigate: (target: ScreenTarget) => void;
}

/** The Reader on one item, read through the port it is given. */
export function ProjectReaderPage({
  port,
  projectId,
  path,
  block,
  navigate,
}: ProjectReaderPageProps) {
  const [proposals, setProposals] = useState(false);
  return (
    <>
      <ReaderScreen
        port={port}
        path={path}
        focusBlockID={block}
        navigate={navigate}
        canNavigate={bowrainOpens}
        onBack={() => navigate({ kind: "map" })}
        backLabel={t("Map")}
        onOpenProposal={() => setProposals(true)}
      />
      {proposals && (
        <SourceProposalsDialog open onOpenChange={setProposals} projectId={projectId} />
      )}
    </>
  );
}

export function ProjectReaderRoute() {
  const { projectId = "", stream = "main", _splat = "" } = useParams({ strict: false });
  const { block } = useSearch({ strict: false }) as { block?: string };
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const scope = { ws: activeWorkspace.slug, projectId, stream };
  const port = useProjectScreensPort(scope);
  const navigate = useScreenNavigate(scope);

  useEffect(() => {
    document.title = `${_splat} · Bowrain`;
  }, [_splat]);

  return (
    <ProjectReaderPage
      port={port}
      projectId={projectId}
      path={_splat}
      block={block}
      navigate={navigate}
    />
  );
}
