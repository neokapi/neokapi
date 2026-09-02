import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { VoiceDashboardView } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

export function ContextDashboardRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Dashboard · Context · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const ws = workspace ?? "";

  return (
    <VoiceDashboardView
      onOpenExperiment={(id) =>
        void navigate({
          to: "/$workspace/context/changes/$id",
          params: { workspace: ws, id },
        })
      }
      onViewExperiments={() =>
        void navigate({ to: "/$workspace/context/changes", params: { workspace: ws } })
      }
      onViewConcepts={() =>
        void navigate({ to: "/$workspace/context/concepts", params: { workspace: ws } })
      }
      onViewVoice={() =>
        void navigate({ to: "/$workspace/context/voice", params: { workspace: ws } })
      }
      onOpenProject={(projectId) =>
        void navigate({
          to: "/$workspace/p/$projectId/s/$stream",
          params: { workspace: ws, projectId, stream: "main" },
        })
      }
    />
  );
}
