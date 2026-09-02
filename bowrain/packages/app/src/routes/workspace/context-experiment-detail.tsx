import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { ExperimentDetailView } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function ExperimentDetailRoute() {
  const navigate = useNavigate();
  const { workspace, id } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Change · Context · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  return (
    <ExperimentDetailView
      changesetId={id ?? ""}
      onBack={() =>
        void navigate({
          to: "/$workspace/context/changes",
          params: { workspace: workspace ?? "" },
        })
      }
      onOpenChangeset={(nextId) =>
        void navigate({
          to: "/$workspace/context/changes/$id",
          params: { workspace: workspace ?? "", id: nextId },
        })
      }
    />
  );
}
