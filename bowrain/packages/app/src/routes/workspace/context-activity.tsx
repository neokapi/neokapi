import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { ActivityView } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function ContextActivityRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Activity · Context · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const ws = workspace ?? "";

  return (
    <ActivityView
      onOpenConcept={(cid) =>
        void navigate({
          to: "/$workspace/context/concepts/$cid",
          params: { workspace: ws, cid },
        })
      }
      onOpenExperiment={(id) =>
        void navigate({
          to: "/$workspace/context/changes/$id",
          params: { workspace: ws, id },
        })
      }
    />
  );
}
