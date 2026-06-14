import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { BrandDashboardView } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function BrandDashboardRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Dashboard — Brand — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  const ws = workspace ?? "";

  return (
    <BrandDashboardView
      onOpenExperiment={(id) =>
        void navigate({
          to: "/$workspace/brand/experiments/$id",
          params: { workspace: ws, id },
        })
      }
      onViewExperiments={() =>
        void navigate({ to: "/$workspace/brand/experiments", params: { workspace: ws } })
      }
      onViewConcepts={() =>
        void navigate({ to: "/$workspace/brand/concepts", params: { workspace: ws } })
      }
      onViewVoice={() =>
        void navigate({ to: "/$workspace/brand/voice", params: { workspace: ws } })
      }
    />
  );
}
