import { useEffect, useState, useCallback } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import {
  CandidateReview,
  BlastRadiusSummary,
  Badge,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  SimpleTooltip,
  useVoiceCandidates,
  usePromoteVoiceRule,
  useRejectVoiceRule,
  useEvaluateVoiceRule,
  useVoiceDrift,
  useVoiceProfile,
  useProjects,
  useUserDisplayNames,
  PageHeader,
} from "@neokapi/ui";
import type { CandidateRule, BlastRadius, DriftResult } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

/**
 * ContextReviewRoute is the correction-learning loop's review surface: the
 * rules a team's repeated corrections suggest for a voice profile, as a list
 * and a detail pane. Promote writes a rule into the workspace's terms as a
 * versioned check, Reject stops it surfacing again. Choose a project in the
 * header to preview a rule's blast radius over that project's content and to
 * see whether its compliance is drifting (AD-019).
 */
export function ContextReviewRoute() {
  const navigate = useNavigate();
  const { activeWorkspace, user } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const { workspace, profileId } = useParams({ strict: false });
  const pid = profileId ?? "";

  const { data: profile } = useVoiceProfile(pid);
  const { data: projects = [] } = useProjects();
  const [projectId, setProjectId] = useState("");
  // Decided rules are part of the list, in closed groups, so who decided what
  // stays one click away.
  const { data: candidates, isLoading, error } = useVoiceCandidates(pid, { all: true });
  const promote = usePromoteVoiceRule(pid);
  const reject = useRejectVoiceRule(pid);
  const evaluate = useEvaluateVoiceRule(pid);
  const { data: drift } = useVoiceDrift(projectId);
  const { nameOf } = useUserDisplayNames();

  const [preview, setPreview] = useState<{ term: string; radius: BlastRadius } | null>(null);
  const [failed, setFailed] = useState<{ term: string; error: unknown } | null>(null);

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Review rules · ${profile?.name ?? "Voice"} · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace, profile?.name]);

  const onPromote = useCallback(
    (c: CandidateRule, replacement?: string) => {
      setFailed(null);
      promote.mutate(
        {
          term: c.term,
          replacement: replacement ?? c.replacement,
          correction_count: c.correction_count,
        },
        { onError: (err) => setFailed({ term: c.term, error: err }) },
      );
    },
    [promote],
  );
  const onReject = useCallback(
    (c: CandidateRule) => {
      setFailed(null);
      reject.mutate(
        { term: c.term, replacement: c.replacement },
        { onError: (err) => setFailed({ term: c.term, error: err }) },
      );
    },
    [reject],
  );
  // Preview impact is only meaningful against a chosen project's content.
  const onEvaluate = useCallback(
    (c: CandidateRule) => {
      if (!projectId) return;
      evaluate.mutate(
        { term: c.term, replacement: c.replacement, project_id: projectId },
        { onSuccess: (radius) => setPreview({ term: c.term, radius }) },
      );
    },
    [evaluate, projectId],
  );

  const busyTerm = promote.isPending
    ? promote.variables?.term
    : reject.isPending
      ? reject.variables?.term
      : evaluate.isPending
        ? evaluate.variables?.term
        : undefined;

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <PageHeader
        className="mb-0"
        eyebrow={profile?.name}
        title="Review suggested rules"
        actions={
          <>
            {projectId && drift?.drifted ? <DriftChip drift={drift} /> : null}
            <label className="flex items-center gap-2 text-sm">
              <span className="text-[11px] text-muted-foreground">Evaluate against</span>
              <select
                value={projectId}
                onChange={(e) => setProjectId(e.target.value)}
                className="rounded-md border bg-background px-2 py-1 text-sm"
              >
                <option value="">a project…</option>
                {projects.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </label>
          </>
        }
      />

      <CandidateReview
        className="min-h-0 flex-1"
        candidates={candidates ?? null}
        loading={isLoading}
        error={error}
        busyTerm={busyTerm}
        failed={failed}
        onPromote={onPromote}
        onReject={onReject}
        onEvaluate={projectId ? onEvaluate : undefined}
        onOpenConcept={(cid) =>
          void navigate({
            to: "/$workspace/context/concepts/$cid",
            params: { workspace: workspace ?? "", cid },
          })
        }
        nameOf={nameOf}
        currentUserId={user?.id}
      />

      <Dialog open={preview !== null} onOpenChange={(open) => !open && setPreview(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Blast radius: promoting &ldquo;{preview?.term}&rdquo;</DialogTitle>
          </DialogHeader>
          {preview ? <BlastRadiusSummary radius={preview.radius} /> : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}

/** The chosen project's compliance drift, as a status with its numbers in the tooltip. */
function DriftChip({ drift }: { drift: DriftResult }) {
  return (
    <SimpleTooltip
      content={
        <>
          Recent average {drift.recent_avg.toFixed(1)}, baseline {drift.baseline_avg.toFixed(1)},
          down {drift.drop.toFixed(1)} over {drift.recent_days} days
          {drift.reason ? `. ${drift.reason}` : ""}
        </>
      }
    >
      <Badge
        variant="warning"
        data-slot="review-drift"
        tabIndex={0}
        className="text-[11px] font-normal"
      >
        Compliance drifting
      </Badge>
    </SimpleTooltip>
  );
}
