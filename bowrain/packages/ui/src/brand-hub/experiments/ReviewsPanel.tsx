// Reviews panel (AD-021): the approve / reject decision on a governed
// change-set, with separation of duties made visible — the author cannot
// approve their own experiment, so we disable approval and say why. Past
// verdicts render as a small thread.
//
// One workspace is exempt: the one with nobody else in it. There the server
// admits the author's own verdict on the `solo_owner` basis, and this panel
// says so both before the decision (the note above the buttons) and after it
// (a badge rendered from the recorded basis), because an approval nobody else
// read must not be able to pass for one that someone did.
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Badge, Button, Card, CardContent, Textarea, cn } from "@neokapi/ui-primitives";
import { Check, X, Shield } from "../../components/icons";
import type { ChangeSetDetail } from "../../types/brand-graph";
import { useApi } from "../../context/ApiContext";
import { useApproveChangeset, useRejectChangeset } from "../../hooks/useChangesetsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { formatRelative } from "../shell/atoms";

function useCurrentUserId(): string | undefined {
  const api = useApi();
  const { data } = useQuery({
    queryKey: ["current-user"],
    queryFn: () => api.getCurrentUser(),
    staleTime: 5 * 60_000,
  });
  return data?.id;
}

export interface ReviewsPanelProps {
  changeset: ChangeSetDetail;
}

export function ReviewsPanel({ changeset }: ReviewsPanelProps) {
  const currentUserId = useCurrentUserId();
  const { nameOf } = useUserDisplayNames();
  const approve = useApproveChangeset(changeset.id);
  const reject = useRejectChangeset(changeset.id);
  const [comment, setComment] = useState("");

  const isReviewing = changeset.status === "in_review";
  const isAuthor = !!currentUserId && currentUserId === changeset.created_by;
  // The server decides this, not the client: it knows the whole membership.
  // Treating it as the single source keeps the button and the rule in step.
  const soloReview = isAuthor && changeset.solo_review;
  const blocked = isAuthor && !soloReview;
  const busy = approve.isPending || reject.isPending;
  const reviewError = approve.error ?? reject.error;

  const submit = (verdict: "approve" | "reject") => {
    const req = comment.trim() ? { comment: comment.trim() } : undefined;
    const mut = verdict === "approve" ? approve : reject;
    mut.mutate(req, { onSuccess: () => setComment("") });
  };

  return (
    <Card>
      <CardContent className="space-y-3 p-4">
        <h3 className="text-sm font-medium">Reviews</h3>

        {changeset.reviews.length === 0 ? (
          <p className="text-sm text-muted-foreground">No reviews yet.</p>
        ) : (
          <ul className="space-y-2">
            {changeset.reviews.map((r, i) => (
              <li key={`${r.reviewer}-${i}`} className="flex items-start gap-2 text-sm">
                {r.verdict === "approve" ? (
                  <Check className="mt-0.5 size-4 text-success" />
                ) : (
                  <X className="mt-0.5 size-4 text-destructive" />
                )}
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{nameOf(r.reviewer)}</span>
                    <span className="text-xs text-muted-foreground">
                      {r.verdict === "approve" ? "approved" : "rejected"} ·{" "}
                      {formatRelative(r.created_at)}
                    </span>
                    {r.basis === "solo_owner" && (
                      <Badge
                        variant="outline"
                        className="gap-1 border-warning/40 text-[10px] text-warning"
                        title="Admitted on the sole-owner basis: recorded by the author because the workspace had no other reviewer"
                      >
                        <Shield className="size-3" />
                        Sole reviewer
                      </Badge>
                    )}
                  </div>
                  {r.comment && <p className="text-sm text-muted-foreground">{r.comment}</p>}
                </div>
              </li>
            ))}
          </ul>
        )}

        {isReviewing && (
          <div className="space-y-2 border-t pt-3">
            {blocked && (
              <p className="flex items-start gap-1.5 rounded-md bg-muted/40 px-2.5 py-2 text-xs text-muted-foreground">
                <Shield className="mt-0.5 size-3.5 shrink-0" />
                You authored this experiment. Approval must come from someone else (separation of
                duties).
              </p>
            )}
            {soloReview && (
              <p className="flex items-start gap-1.5 rounded-md bg-warning/10 px-2.5 py-2 text-xs text-muted-foreground">
                <Shield className="mt-0.5 size-3.5 shrink-0 text-warning" />
                You authored this experiment and are the workspace&rsquo;s only reviewer, so you may
                decide it yourself. The verdict will be recorded as reviewed by its author. Invite a
                second reviewer and this stops being possible.
              </p>
            )}
            <Textarea
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              rows={2}
              placeholder="Add a review comment (optional)…"
              className="text-sm"
            />
            {reviewError != null && (
              <p className="text-sm text-destructive">
                {reviewError instanceof Error
                  ? reviewError.message
                  : "Could not record the review."}
              </p>
            )}
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                onClick={() => submit("approve")}
                disabled={busy || blocked}
                title={blocked ? "You cannot approve your own experiment" : undefined}
                className={cn(blocked && "cursor-not-allowed")}
              >
                <Check />
                {soloReview ? "Approve as sole reviewer" : "Approve"}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => submit("reject")}
                disabled={busy || blocked}
                title={blocked ? "You cannot reject your own experiment" : undefined}
                className={cn(blocked && "cursor-not-allowed")}
              >
                <X />
                Reject
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
