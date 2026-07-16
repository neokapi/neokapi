import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { ConceptsSection } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function ConceptsRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Concepts — Brand — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  return (
    <ConceptsSection
      onOpenConcept={(cid) =>
        void navigate({
          to: "/$workspace/brand/concepts/$cid",
          params: { workspace: workspace ?? "", cid },
        })
      }
    />
  );
}
