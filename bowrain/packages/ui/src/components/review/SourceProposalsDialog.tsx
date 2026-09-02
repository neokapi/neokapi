import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  Button,
  Badge,
} from "@neokapi/ui-primitives";
import { useSourceProposals, useDecideSourceProposal } from "../../hooks/useSourceProposalsApi";
import { ErrorNotice } from "../../errors";
import { Check, X, CircleCheck } from "../icons";
import type { SourceProposal } from "../../types/api";

export interface SourceProposalsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  localeName?: (locale: string) => string;
  onDone?: (message: string) => void;
}

/**
 * SourceProposalsDialog is the source owner's review surface for back-to-source
 * proposals (RV-F). It lists the open proposals with the original beside the
 * proposed source, the finder and the language they were reviewing, and the
 * rationale, and it lets a PermEditSource owner approve each one (apply, then
 * re-draft every language) or reject it.
 * Approval is gated server-side, so a non-owner viewing this surface gets a clear
 * error rather than a silent apply.
 */
export function SourceProposalsDialog({
  open,
  onOpenChange,
  projectId,
  localeName,
  onDone,
}: SourceProposalsDialogProps) {
  const { data: proposals = [], isLoading } = useSourceProposals(projectId, open);
  const decide = useDecideSourceProposal(projectId);
  const [pendingId, setPendingId] = useState<string | null>(null);

  const act = (p: SourceProposal, decision: "approve" | "reject") => {
    setPendingId(p.id);
    decide.mutate(
      { proposalId: p.id, decision },
      {
        onSuccess: () => {
          onDone?.(
            decision === "approve"
              ? "Source change approved — re-drafting the affected locales."
              : "Source change rejected.",
          );
        },
        onSettled: () => setPendingId(null),
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl" data-testid="source-proposals-dialog">
        <DialogHeader>
          <DialogTitle>Source change proposals</DialogTitle>
          <DialogDescription>
            Reviewers proposed these fixes to the source text. Approving a change rewrites the
            source and re-drafts every locale; rejecting closes it.
          </DialogDescription>
        </DialogHeader>

        {decide.error && (
          <ErrorNotice error={decide.error} title="Couldn't apply the decision" variant="inline" />
        )}

        <div className="max-h-[60vh] space-y-3 overflow-auto">
          {isLoading ? (
            <p className="py-6 text-center text-sm text-muted-foreground">Loading…</p>
          ) : proposals.length === 0 ? (
            <div
              className="flex flex-col items-center gap-2 py-8 text-center text-sm text-muted-foreground"
              data-testid="source-proposals-empty"
            >
              <CircleCheck className="h-6 w-6 text-success" />
              No open source proposals.
            </div>
          ) : (
            proposals.map((p) => (
              <div
                key={p.id}
                className="space-y-2 rounded-lg border border-border p-3"
                data-testid="source-proposal-item"
              >
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <span className="truncate font-medium text-foreground" title={p.item_name}>
                    {p.item_name || p.block_id}
                  </span>
                  {p.found_in_locale && (
                    <Badge variant="outline" className="font-normal">
                      found in {localeName ? localeName(p.found_in_locale) : p.found_in_locale}
                    </Badge>
                  )}
                  {p.kind === "mark-dnt" && (
                    <Badge variant="outline" className="font-normal">
                      do-not-translate
                    </Badge>
                  )}
                </div>

                <div className="grid gap-2 text-sm sm:grid-cols-2">
                  <div className="rounded border border-border bg-muted/20 p-2">
                    <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                      Current
                    </div>
                    <span className="text-muted-foreground line-through">{p.original_source}</span>
                  </div>
                  <div className="rounded border border-success/30 bg-success/5 p-2">
                    <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-success">
                      Proposed
                    </div>
                    <span>{p.proposed_source}</span>
                  </div>
                </div>

                {p.rationale && (
                  <p className="text-xs text-muted-foreground">
                    <span className="font-medium">Why:</span> {p.rationale}
                  </p>
                )}

                <div className="flex items-center justify-end gap-2">
                  <Button
                    size="sm"
                    variant="destructive"
                    disabled={pendingId === p.id}
                    onClick={() => act(p, "reject")}
                    data-testid="source-proposal-reject"
                  >
                    <X className="mr-1 h-4 w-4" /> Reject
                  </Button>
                  <Button
                    size="sm"
                    variant="success"
                    disabled={pendingId === p.id}
                    onClick={() => act(p, "approve")}
                    data-testid="source-proposal-approve"
                  >
                    <Check className="mr-1 h-4 w-4" /> Approve &amp; re-draft
                  </Button>
                </div>
              </div>
            ))
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
