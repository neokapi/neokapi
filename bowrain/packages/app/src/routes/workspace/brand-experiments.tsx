import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { ExperimentsView } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function ExperimentsRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Experiments — Brand — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  return (
    <ExperimentsView
      onOpenExperiment={(id) =>
        void navigate({
          to: "/$workspace/brand/experiments/$id",
          params: { workspace: workspace ?? "", id },
        })
      }
    />
  );
}
