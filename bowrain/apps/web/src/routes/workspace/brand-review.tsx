import { useEffect, useState, useCallback } from "react";
import { useParams, useRouteContext } from "@tanstack/react-router";
import {
  CandidateRulesList,
  BlastRadiusSummary,
  DriftAlert,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  useBrandCandidates,
  usePromoteBrandRule,
  useRejectBrandRule,
  useEvaluateBrandRule,
  useBrandDrift,
  useBrandProfile,
  useProjects,
} from "@neokapi/ui";
import type { CandidateRule, BlastRadius } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

/**
 * BrandReviewRoute is the correction-learning loop's review surface: the
 * candidate rules a team's corrections have produced for a profile, with
 * Promote / Reject. Pick a project to also preview a candidate's blast radius
 * across that project's content and to see its compliance-drift status (AD-019).
 */
export function BrandReviewRoute() {
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const { profileId } = useParams({ strict: false });
  const pid = profileId ?? "";

  const { data: profile } = useBrandProfile(pid);
  const { data: projects = [] } = useProjects();
  const [projectId, setProjectId] = useState("");
  const [showHistory, setShowHistory] = useState(false);
  const { data: candidates = [], isLoading } = useBrandCandidates(pid, { all: showHistory });
  const promote = usePromoteBrandRule(pid);
  const reject = useRejectBrandRule(pid);
  const evaluate = useEvaluateBrandRule(pid);
  const { data: drift } = useBrandDrift(projectId);

  const [preview, setPreview] = useState<{ term: string; radius: BlastRadius } | null>(null);

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Review rules — ${profile?.name ?? "Brand"} — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace, profile?.name]);

  const onPromote = useCallback(
    (c: CandidateRule) => {
      promote.mutate({ term: c.term, replacement: c.replacement, correction_count: c.correction_count });
    },
    [promote],
  );
  const onReject = useCallback(
    (c: CandidateRule) => {
      reject.mutate({ term: c.term, replacement: c.replacement });
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
    <div className="mx-auto max-w-3xl space-y-4 p-6">
      <header className="space-y-1">
        <h1 className="text-xl font-semibold">Review suggested rules</h1>
        <p className="text-sm text-muted-foreground">
          Brand checks act like tests for AI output. Repeated corrections become candidate rules —
          promote one to harden it into a check on every future generation, or reject it to stop it
          re-surfacing.
        </p>
      </header>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <label className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Evaluate against</span>
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
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={showHistory}
            onChange={(e) => setShowHistory(e.target.checked)}
          />
          Show decided
        </label>
      </div>

      {projectId && drift ? <DriftAlert drift={drift} /> : null}

      {isLoading ? (
        <div className="py-8 text-center text-sm text-muted-foreground">Loading candidates…</div>
      ) : (
        <CandidateRulesList
          candidates={candidates}
          onPromote={onPromote}
          onReject={onReject}
          onEvaluate={projectId ? onEvaluate : undefined}
          busyTerm={busyTerm}
        />
      )}

      <Dialog open={preview !== null} onOpenChange={(open) => !open && setPreview(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Blast radius — promoting &ldquo;{preview?.term}&rdquo;</DialogTitle>
          </DialogHeader>
          {preview ? <BlastRadiusSummary radius={preview.radius} /> : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
