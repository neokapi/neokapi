import { Badge, Button, cn } from "@neokapi/ui-primitives";
import type { CandidateRule, RuleDecisionStatus } from "./types";

interface CandidateRulesListProps {
  candidates: CandidateRule[];
  /** Promote a candidate into the profile as an enforced rule. */
  onPromote?: (candidate: CandidateRule) => void;
  /** Reject a candidate so it stops re-surfacing. */
  onReject?: (candidate: CandidateRule) => void;
  /** Preview the blast radius of promoting a candidate. */
  onEvaluate?: (candidate: CandidateRule) => void;
  /** Term currently being acted on — disables its row actions. */
  busyTerm?: string;
  className?: string;
}

const statusStyles: Record<RuleDecisionStatus, string> = {
  pending: "bg-muted text-muted-foreground",
  approved: "bg-primary/10 text-primary",
  rejected: "bg-muted text-muted-foreground line-through",
  promoted: "bg-success/10 text-success dark:bg-success/20",
};

function StatusBadge({ candidate }: { candidate: CandidateRule }) {
  const label =
    candidate.status === "promoted" && candidate.auto ? "auto-promoted" : candidate.status;
  return (
    <Badge className={cn("shrink-0 text-[10px] capitalize", statusStyles[candidate.status])}>
      {label}
      {candidate.status === "promoted" && candidate.promoted_version
        ? ` · v${candidate.promoted_version}`
        : ""}
    </Badge>
  );
}

/**
 * CandidateRulesList is the loop's review surface: the candidate rules a team's
 * corrections have produced, each with the count of corrections behind it and
 * its decision. Pending candidates offer Promote / Reject / Preview impact;
 * resolved candidates (promoted or rejected) read as history.
 */
export function CandidateRulesList({
  candidates,
  onPromote,
  onReject,
  onEvaluate,
  busyTerm,
  className,
}: CandidateRulesListProps) {
  if (candidates.length === 0) {
    return (
      <div className={cn("text-sm text-muted-foreground text-center py-6", className)}>
        No candidate rules yet. As your team corrects content, repeated corrections become
        candidates here.
      </div>
    );
  }

  return (
    <ul className={cn("space-y-2", className)}>
      {candidates.map((candidate) => {
        const actionable = candidate.status === "pending" || candidate.status === "approved";
        const busy = busyTerm === candidate.term;
        return (
          <li
            key={candidate.term}
            className="flex flex-wrap items-center gap-3 rounded-md border p-3 text-sm bg-card/50"
          >
            <div className="flex-1 min-w-[12rem] space-y-1">
              <p className="font-medium">
                <span className="text-destructive line-through">{candidate.term}</span>
                {candidate.replacement && (
                  <>
                    {" → "}
                    <span className="text-success">{candidate.replacement}</span>
                  </>
                )}
              </p>
              <div className="flex flex-wrap gap-2 text-[10px] text-muted-foreground">
                <span>
                  {candidate.correction_count} correction
                  {candidate.correction_count === 1 ? "" : "s"}
                </span>
                <span className="capitalize">{candidate.dimension.replace("_", " ")}</span>
              </div>
            </div>
            <StatusBadge candidate={candidate} />
            {actionable && (
              <div className="flex gap-2">
                {onEvaluate && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    onClick={() => onEvaluate(candidate)}
                  >
                    Preview impact
                  </Button>
                )}
                {onReject && (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => onReject(candidate)}
                  >
                    Reject
                  </Button>
                )}
                {onPromote && (
                  <Button size="sm" disabled={busy} onClick={() => onPromote(candidate)}>
                    Promote
                  </Button>
                )}
              </div>
            )}
          </li>
        );
      })}
    </ul>
  );
}
