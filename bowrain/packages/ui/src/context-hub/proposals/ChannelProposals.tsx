import { Badge, Button, Card, cn } from "@neokapi/ui-primitives";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import { formatRelative } from "../shell/atoms";
import { Check, Link, X } from "../../components/icons";
import { useWorkspace } from "../../context/WorkspaceContext";
import type { ChannelAliasProposal } from "../../types/channel-proposals";
import {
  channelProposalKey,
  useChannelProposals,
  useJudgeChannelProposal,
} from "./useChannelProposals";

/**
 * The workspace's channel-slug equivalence proposals.
 *
 * A project cannot see this: a recipe declares its own channels and resolves
 * them offline, and nothing inside it knows the workspace's other project
 * spells the same surface differently. The workspace can see it, so it says so
 * — and stops there. Judging records agreement or disagreement between people;
 * it rewrites no slug in any recipe, because a project must resolve its
 * coordinates the same way whether or not it has ever been connected.
 */
export function ChannelProposalsPanel({ className }: { className?: string }) {
  const { data, isLoading, error } = useChannelProposals();
  const { activeWorkspace } = useWorkspace();
  const judge = useJudgeChannelProposal();

  const canJudge = activeWorkspace?.role === "owner" || activeWorkspace?.role === "admin";
  const proposals = data?.proposals ?? [];
  const open = proposals.filter((p) => (p.status ?? "proposed") === "proposed");
  const judged = proposals.filter((p) => (p.status ?? "proposed") !== "proposed");

  // The listing is empty for most workspaces most of the time, and an empty
  // governance section is noise on a hub about profiles — so is a skeleton
  // standing in for one. The section appears when there is something to say.
  if (isLoading || error || proposals.length === 0) return null;

  return (
    <section className={cn("space-y-3", className)} data-testid="channel-proposals">
      <div className="space-y-1">
        <h2 className="flex items-center gap-2 text-sm font-medium text-foreground">
          <Link className="size-4 text-muted-foreground" />
          Channel names
        </h2>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Two projects spell what looks like one channel differently. Accepting records that they
          name the same surface; it changes no recipe, because each project resolves its own
          coordinates.
        </p>
      </div>

      <Card className="divide-y divide-border p-0">
        {open.map((p) => (
          <ProposalRow
            key={channelProposalKey(p)}
            proposal={p}
            canJudge={canJudge}
            pending={judge.isPending && judge.variables?.proposed_channel === p.proposed_channel}
            onJudge={(status) =>
              judge.mutate({
                profile: p.profile,
                proposed_channel: p.proposed_channel,
                existing_channel: p.existing_channel,
                status,
              })
            }
          />
        ))}
        {judged.map((p) => (
          <ProposalRow key={channelProposalKey(p)} proposal={p} canJudge={false} />
        ))}
      </Card>

      {open.length === 0 && (
        <p className="text-sm text-muted-foreground">
          <Plural count={judged.length}>
            <One>The one pair the workspace raised has been judged.</One>
            <Other>Every pair the workspace raised has been judged.</Other>
          </Plural>
        </p>
      )}

      {judge.error && (
        <p className="text-sm text-destructive">
          {judge.error instanceof Error ? judge.error.message : "The judgement was not recorded."}
        </p>
      )}
    </section>
  );
}

/**
 * One proposal, read as the claim it makes: two names, the reason they look
 * alike, and the two things a reviewer can say about it.
 */
function ProposalRow({
  proposal,
  canJudge,
  pending,
  onJudge,
}: {
  proposal: ChannelAliasProposal;
  canJudge: boolean;
  pending?: boolean;
  onJudge?: (status: "accepted" | "dismissed") => void;
}) {
  const status = proposal.status ?? "proposed";

  return (
    <div className="flex flex-wrap items-start justify-between gap-4 p-4">
      <div className="min-w-0 flex-1 space-y-1.5">
        <p className="text-sm text-foreground">
          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm">
            {proposal.proposed_channel}
          </code>
          <span className="px-2 text-muted-foreground">and</span>
          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm">
            {proposal.existing_channel}
          </code>
          <span className="pl-2 text-muted-foreground">look like one channel.</span>
        </p>
        {proposal.evidence && (
          <p className="text-sm text-muted-foreground">Because {proposal.evidence}.</p>
        )}
        <p className="text-xs text-muted-foreground">
          {proposal.profile && (
            <>
              Under <span className="font-mono">{proposal.profile}</span>
              <span className="px-1.5 opacity-50">·</span>
            </>
          )}
          {proposal.collection ? (
            <>
              Seen in <span className="font-mono">{proposal.collection}</span>
              <span className="px-1.5 opacity-50">·</span>
            </>
          ) : null}
          {status === "proposed" ? (
            formatRelative(proposal.updated_at)
          ) : (
            <>Judged {formatRelative(proposal.judged_at || proposal.updated_at)}</>
          )}
        </p>
      </div>

      <div className="flex shrink-0 items-center gap-2">
        {status === "accepted" && (
          <Badge className="border-transparent bg-success/15 text-success">One channel</Badge>
        )}
        {status === "dismissed" && <Badge variant="outline">Two channels</Badge>}
        {status === "proposed" && canJudge && onJudge && (
          <>
            <Button
              variant="outline"
              size="sm"
              disabled={pending}
              onClick={() => onJudge("dismissed")}
            >
              <X className="mr-1.5 size-3.5" /> Two channels
            </Button>
            <Button size="sm" disabled={pending} onClick={() => onJudge("accepted")}>
              <Check className="mr-1.5 size-3.5" /> One channel
            </Button>
          </>
        )}
        {status === "proposed" && !canJudge && (
          <Badge variant="outline" className="font-normal">
            Waiting on a workspace admin
          </Badge>
        )}
      </div>
    </div>
  );
}
