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
      document.title = `Experiment — Brand — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  return (
    <ExperimentDetailView
      changesetId={id ?? ""}
      onBack={() =>
        void navigate({
          to: "/$workspace/brand/experiments",
          params: { workspace: workspace ?? "" },
        })
      }
    />
  );
}
