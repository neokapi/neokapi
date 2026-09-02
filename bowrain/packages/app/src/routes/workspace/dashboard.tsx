import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useQuery, useSuspenseQuery, useQueryClient } from "@tanstack/react-query";
import {
  ProjectDashboard,
  ConfirmDialog,
  useApi,
  useAnalytics,
  AnalyticsEvents,
  type LoopStatusData,
  type ProjectInfo,
} from "@neokapi/ui";
import {
  activitiesQueryOptions,
  myReviewTaskCountsQueryOptions,
  projectsQueryOptions,
} from "../../queries";
import { usePlatform } from "../../platform";
import type { WorkspaceRouteContext } from "..";

/**
 * Activity types the loop itself produces (flows/jobs, ship gates, pushes,
 * translation and review movement, connector syncs). The loop-status layer
 * surfaces the newest of these from the workspace feed the top bar already
 * fetches — member/stream/task admin noise stays out.
 */
const LOOP_ACTIVITY_TYPES = new Set([
  "flow.completed",
  "flow.failed",
  "job.completed",
  "job.failed",
  "extraction.completed",
  "gate.passed",
  "gate.failed",
  "item.pushed",
  "item.pulled",
  "block.translated",
  "block.reviewed",
  "review.assigned",
  "review.decided",
  "connector.synced",
  "connector.failed",
]);

export function ProjectDashboardRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const adapter = useApi();
  const platform = usePlatform();
  const { capture } = useAnalytics();
  const queryClient = useQueryClient();
  const { activeWorkspace, contextScanAvailable } = useRouteContext({
    strict: false,
  }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;

  useEffect(() => {
    document.title = `${activeWorkspace.name} · Bowrain`;
  }, [activeWorkspace.name]);

  const { data: projects } = useSuspenseQuery(projectsQueryOptions(adapter, ws));
  const hasProjects = projects.length > 0;

  // Loop-status inputs. Activities reuses the layout's top-bar query (same
  // key — a cache hit, no extra request, no extra polling). The voice rollup
  // and the review-task count are the two additional fetches on this surface;
  // both are skipped entirely on first run.
  //
  // These are non-suspense enrichments: a failure leaves `data` undefined and
  // the dependent card simply doesn't render. Silent absence is intended here —
  // only the primary projects query above is suspense-backed and errors into
  // the route boundary; the loop-status cards are secondary.
  const { data: activitiesData } = useQuery({
    ...activitiesQueryOptions(adapter, ws),
    enabled: hasProjects,
  });
  // Open review work assigned to the caller, filtered and counted server-side
  // — the same query the review inbox lists, so the card's number and that
  // page's rows cannot disagree.
  const { data: reviewTaskCounts } = useQuery({
    ...myReviewTaskCountsQueryOptions(adapter, ws),
    enabled: hasProjects,
  });
  const { data: voiceRollup } = useQuery({
    queryKey: ["voice-rollup", ws, {}],
    queryFn: () => adapter.getVoiceRollup(ws),
    enabled: hasProjects,
    staleTime: 30_000,
  });
  // The workspace loop rollup (one server aggregate): latest convergence run
  // + ship-state rollup — the two cards a per-project fan-out would have cost
  // up to max-projects requests each. No polling: the SSE invalidation
  // planner refetches this key on convergence.* events.
  const { data: loopRollup } = useQuery({
    queryKey: ["loopRollup", ws],
    queryFn: () => adapter.getLoopRollup(ws),
    enabled: hasProjects,
    staleTime: 30_000,
  });

  const loopStatus = useMemo<LoopStatusData | undefined>(() => {
    if (!hasProjects) return undefined;

    const latest = activitiesData?.activities?.find((a) => LOOP_ACTIVITY_TYPES.has(a.type));
    const openReviewTasks = reviewTaskCounts?.total;

    let brand: LoopStatusData["brand"];
    if (voiceRollup) {
      const scored = voiceRollup.projects.filter((p) => p.overall !== null);
      brand = {
        averageScore: scored.length
          ? Math.round(scored.reduce((acc, p) => acc + (p.overall ?? 0), 0) / scored.length)
          : null,
        scoredProjects: scored.length,
        driftingProjects: voiceRollup.projects.filter(
          (p) => p.drift?.drifted === true || p.trend === "down",
        ).length,
      };
    }

    // The two rollup-fed slots stay absent (cards hidden) until the server
    // reports data — no runs yet / no cached dashboard rollups to fold.
    let latestRun: LoopStatusData["latestRun"];
    if (loopRollup?.latest_run) {
      const run = loopRollup.latest_run;
      latestRun = {
        state: run.state,
        stallReason: run.stall_reason,
        projectName: run.project_name,
        updatedAt: run.updated_at,
      };
    }
    let ship: LoopStatusData["ship"];
    if (loopRollup?.ship) {
      const rollup = loopRollup.ship;
      ship = {
        governed: rollup.governed,
        aiShippable: rollup.ai_shippable,
        pending: rollup.pending,
        countedProjects: rollup.counted_projects,
        totalProjects: rollup.total_projects,
      };
    }

    return {
      latestActivity: latest
        ? { summary: latest.summary, created_at: latest.created_at }
        : undefined,
      latestRun,
      openReviewTasks,
      ship,
      brand,
    };
  }, [hasProjects, activitiesData, reviewTaskCounts, voiceRollup, loopRollup]);

  const invalidateProjects = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["projects", ws] });
  }, [queryClient, ws]);

  const handleCreateProject = useCallback(
    async (name: string, sourceLang: string, targetLangs: string[], files?: File[]) => {
      const info = await adapter.createProject(ws, name, sourceLang, targetLangs);
      // Client-side complement of the server's project_created; no name here.
      capture(AnalyticsEvents.projectCreateSubmitted, {
        source_language: sourceLang,
        target_count: targetLangs.length,
        file_count: files?.length ?? 0,
      });
      if (files && files.length > 0) {
        // The first-run drop affordance: reuse the upload-after-create flow.
        // Fail-soft — the project exists either way, and the project page
        // offers the same upload again.
        try {
          await adapter.uploadFiles(ws, info.id, files);
        } catch {
          // Surfaced on the project page when the user retries the upload.
        }
      }
      invalidateProjects();
      void navigate({
        to: "/$workspace/p/$projectId/s/$stream",
        params: { workspace: workspace ?? ws, projectId: info.id, stream: "main" },
      });
    },
    [ws, workspace, adapter, capture, navigate, invalidateProjects],
  );

  const handleOpenProject = useCallback(
    (project: ProjectInfo) => {
      void navigate({
        to: "/$workspace/p/$projectId/s/$stream",
        params: { workspace: workspace ?? ws, projectId: project.id, stream: "main" },
      });
    },
    [navigate, workspace, ws],
  );

  const handleCreateSampleProject = useCallback(async () => {
    const info = await adapter.createProject(ws, "Sample Project", "en", ["fr", "de", "ja"]);
    capture(AnalyticsEvents.projectCreateSubmitted, {
      source_language: "en",
      target_count: 3,
      sample: true,
    });
    invalidateProjects();
    void navigate({
      to: "/$workspace/p/$projectId/s/$stream",
      params: { workspace: workspace ?? ws, projectId: info.id, stream: "main" },
    });
  }, [ws, workspace, adapter, capture, navigate, invalidateProjects]);

  const handleEditProject = useCallback(
    async (projectId: string, data: { name?: string; target_languages?: string[] }) => {
      await adapter.updateProject(ws, projectId, data);
      invalidateProjects();
    },
    [ws, adapter, invalidateProjects],
  );

  const handleScanVoice = useCallback(() => {
    void navigate({
      to: "/$workspace/context/scan",
      params: { workspace: workspace ?? ws },
    });
  }, [navigate, workspace, ws]);

  const handleInviteTeam = useCallback(() => {
    void navigate({
      to: "/$workspace/settings/members",
      params: { workspace: workspace ?? ws },
    });
  }, [navigate, workspace, ws]);

  const handleOpenActivities = useCallback(() => {
    void navigate({
      to: "/$workspace/activities",
      params: { workspace: workspace ?? ws },
    });
  }, [navigate, workspace, ws]);

  const handleOpenTasks = useCallback(() => {
    void navigate({
      to: "/$workspace/tasks",
      params: { workspace: workspace ?? ws },
    });
  }, [navigate, workspace, ws]);

  // The "Awaiting review" card opens the workspace review inbox — pending
  // review across every project, not just what's assigned to me.
  const handleOpenReviewInbox = useCallback(() => {
    void navigate({
      to: "/$workspace/review-inbox",
      params: { workspace: workspace ?? ws },
    });
  }, [navigate, workspace, ws]);

  // Deep link for the latest-run card: the run's project's runs page.
  const handleOpenRuns = useCallback(() => {
    const run = loopRollup?.latest_run;
    if (!run) return;
    void navigate({
      to: "/$workspace/p/$projectId/s/$stream/runs",
      params: {
        workspace: workspace ?? ws,
        projectId: run.project_id,
        stream: run.stream || "main",
      },
    });
  }, [navigate, workspace, ws, loopRollup]);

  // Deep link for the ship card: the overview — where the delivery band lives —
  // of the counted project with the most pending locales (the server sorts it
  // first).
  const handleOpenDelivery = useCallback(() => {
    const target = loopRollup?.ship?.projects?.[0];
    if (!target) return;
    void navigate({
      to: "/$workspace/p/$projectId/s/$stream",
      params: {
        workspace: workspace ?? ws,
        projectId: target.project_id,
        stream: target.stream || "main",
      },
    });
  }, [navigate, workspace, ws, loopRollup]);

  const handleOpenVoiceDashboard = useCallback(() => {
    void navigate({
      to: "/$workspace/context/dashboard",
      params: { workspace: workspace ?? ws },
    });
  }, [navigate, workspace, ws]);

  const [archiveProjectId, setArchiveProjectId] = useState<string | null>(null);
  const confirmArchiveProject = useCallback(async () => {
    if (!archiveProjectId) return;
    await adapter.deleteProject(ws, archiveProjectId);
    setArchiveProjectId(null);
    invalidateProjects();
  }, [ws, adapter, archiveProjectId, invalidateProjects]);

  return (
    <>
      <ProjectDashboard
        projects={projects}
        onCreateProject={handleCreateProject}
        onOpenProject={handleOpenProject}
        onCreateSampleProject={handleCreateSampleProject}
        workspaceName={activeWorkspace.name}
        onEditProject={handleEditProject}
        onArchiveProject={setArchiveProjectId}
        workspaceLanguages={activeWorkspace.languages}
        // Hosted-scan CTA only where the server runs the brand-scan job system.
        onScanVoice={contextScanAvailable ? handleScanVoice : undefined}
        onInviteTeam={handleInviteTeam}
        serverUrl={platform.kind === "web" ? window.location.origin : undefined}
        loopStatus={loopStatus}
        onOpenActivities={handleOpenActivities}
        onOpenRuns={handleOpenRuns}
        onOpenTasks={handleOpenTasks}
        onOpenReview={handleOpenReviewInbox}
        onOpenDelivery={handleOpenDelivery}
        onOpenVoiceDashboard={handleOpenVoiceDashboard}
      />

      <ConfirmDialog
        open={archiveProjectId !== null}
        onOpenChange={(v) => {
          if (!v) setArchiveProjectId(null);
        }}
        title="Archive project"
        description="This project will be moved to the Recycle Bin. You can restore it at any time."
        confirmLabel="Archive"
        variant="destructive"
        onConfirm={confirmArchiveProject}
      />
    </>
  );
}
