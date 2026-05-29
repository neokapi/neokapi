import { useEffect, useState, useCallback } from "react";
import { useParams, useRouteContext } from "@tanstack/react-router";
import {
  CandidateRulesList,
  useBrandCandidates,
  usePromoteBrandRule,
  useRejectBrandRule,
  useBrandProfile,
} from "@neokapi/ui";
import type { CandidateRule } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

/**
 * BrandReviewRoute is the correction-learning loop's review surface: the
 * candidate rules a team's corrections have produced for a profile, with
 * Promote / Reject. Promoting a candidate turns a repeated correction into a
 * versioned, enforced check (AD-019).
 */
export function BrandReviewRoute() {
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const { profileId } = useParams({ strict: false });
  const pid = profileId ?? "";

  const { data: profile } = useBrandProfile(pid);
  const [showHistory, setShowHistory] = useState(false);
  const { data: candidates = [], isLoading } = useBrandCandidates(pid, { all: showHistory });
  const promote = usePromoteBrandRule(pid);
  const reject = useRejectBrandRule(pid);

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

  const busyTerm = promote.isPending
    ? promote.variables?.term
    : reject.isPending
      ? reject.variables?.term
      : undefined;

  return (
    <div className="mx-auto max-w-3xl space-y-4 p-6">
      <header className="space-y-1">
        <h1 className="text-xl font-semibold">Review suggested rules</h1>
        <p className="text-sm text-muted-foreground">
          Repeated corrections become candidate rules. Promote a candidate to enforce it on every
          future generation, or reject it to stop it re-surfacing.
        </p>
      </header>

      <div className="flex justify-end">
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={showHistory}
            onChange={(e) => setShowHistory(e.target.checked)}
          />
          Show decided
        </label>
      </div>

      {isLoading ? (
        <div className="py-8 text-center text-sm text-muted-foreground">Loading candidates…</div>
      ) : (
        <CandidateRulesList
          candidates={candidates}
          onPromote={onPromote}
          onReject={onReject}
          busyTerm={busyTerm}
        />
      )}
    </div>
  );
}
