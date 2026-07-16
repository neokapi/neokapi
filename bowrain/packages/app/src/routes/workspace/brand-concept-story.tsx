import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { ConceptStorySection } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function ConceptStoryRoute() {
  const navigate = useNavigate();
  const { workspace, cid } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Concept — Brand — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  const ws = workspace ?? "";

  return (
    <ConceptStorySection
      conceptId={cid ?? ""}
      onBack={() => void navigate({ to: "/$workspace/brand/concepts", params: { workspace: ws } })}
      onOpenConcept={(nextCid) =>
        void navigate({
          to: "/$workspace/brand/concepts/$cid",
          params: { workspace: ws, cid: nextCid },
        })
      }
      onOpenExperiments={() =>
        void navigate({ to: "/$workspace/brand/experiments", params: { workspace: ws } })
      }
    />
  );
}
