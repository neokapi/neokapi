import { useCallback, useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { ConceptsSection } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

/**
 * ConceptsRoute is the workspace's concepts as a list beside a detail pane. It
 * renders at /concepts and at /concepts/$cid, and the segment is the selected
 * concept: a link from Activity, the dashboard or a review opens the list with
 * that concept selected, and selecting another rewrites the segment in place.
 */
export function ConceptsRoute() {
  const navigate = useNavigate();
  const { workspace, cid } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = workspace ?? "";

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Concepts · Context · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const onSelect = useCallback(
    (next: string) =>
      void navigate({
        to: "/$workspace/context/concepts/$cid",
        params: { workspace: ws, cid: next },
        // The selection is not a page of its own: Back leaves the list rather
        // than stepping through every concept the keyboard passed over.
        replace: true,
      }),
    [navigate, ws],
  );

  return (
    <ConceptsSection
      selectedId={cid ?? null}
      onSelect={onSelect}
      onOpenChangeSet={(id) =>
        void navigate({
          to: "/$workspace/context/changes/$id",
          params: { workspace: ws, id },
        })
      }
      onOpenExperiments={() =>
        void navigate({ to: "/$workspace/context/changes", params: { workspace: ws } })
      }
    />
  );
}
