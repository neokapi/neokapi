import { Card, CardContent, Button, cn } from "@neokapi/ui-primitives";
import { Eye, CircleCheck, Sparkles, ShieldCheck, ArrowRight } from "../icons";

/** One project's pending-review standing in the workspace review inbox. */
export interface ReviewInboxProject {
  projectId: string;
  projectName: string;
  /** The project's default stream — where its review session lives. */
  stream: string;
  /** Project-locales awaiting review (pending ship state). */
  pending: number;
  /** Governed (human-approved) project-locales. */
  governed: number;
  /** AI-shippable (machine-reviewed only) project-locales. */
  aiShippable: number;
}

export interface ReviewInboxProps {
  /** Projects with pending review, most-pending first. */
  projects: ReviewInboxProject[];
  /** Open a project's review session. */
  onOpenReview: (projectId: string, stream: string) => void;
  loading?: boolean;
  /** Coverage note when the rollup counts only some projects. */
  coverageNote?: string;
}

/**
 * ReviewInbox is the workspace-level roll-up of pending review: every project
 * that has blocks awaiting review, most-pending first, each linking into its
 * own focused review session. It reads the loop rollup's per-project ship
 * standing (project-scoped, not assignee-scoped), so pending review is visible
 * across the workspace and one click away from being cleared.
 */
export function ReviewInbox({ projects, onOpenReview, loading, coverageNote }: ReviewInboxProps) {
  const withPending = projects.filter((p) => p.pending > 0);

  return (
    <div className="mx-auto w-full max-w-4xl p-4 md:p-6" data-testid="review-inbox">
      <header className="mb-4 space-y-1">
        <h1 className="flex items-center gap-2 text-lg font-semibold">
          <Eye className="h-5 w-5 text-primary" /> Review inbox
        </h1>
        <p className="text-sm text-muted-foreground">
          Projects with translations awaiting review. Open a project to clear its pending work in
          one focused session.
        </p>
      </header>

      {loading ? (
        <div className="py-10 text-center text-sm text-muted-foreground">
          Loading review standing…
        </div>
      ) : withPending.length === 0 ? (
        <Card data-testid="review-inbox-empty">
          <CardContent className="flex flex-col items-center gap-2 py-10 text-center">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-success/10 text-success">
              <CircleCheck className="h-6 w-6" />
            </div>
            <p className="text-sm font-medium">Nothing awaiting review</p>
            <p className="max-w-sm text-sm text-muted-foreground">
              Every project is either fully reviewed or has no translated work pending. New review
              lands here as the loop produces it.
            </p>
          </CardContent>
        </Card>
      ) : (
        <ul className="space-y-2" data-testid="review-inbox-list">
          {withPending.map((p) => (
            <li key={`${p.projectId}:${p.stream}`}>
              <Card
                className={cn("cursor-pointer transition-colors hover:border-primary/40")}
                onClick={() => onOpenReview(p.projectId, p.stream)}
                data-testid={`review-inbox-project-${p.projectId}`}
              >
                <CardContent className="flex items-center gap-4 py-3">
                  <div className="flex items-baseline gap-2">
                    <span className="text-2xl font-semibold tabular-nums leading-none text-primary">
                      {p.pending}
                    </span>
                    <span className="text-xs text-muted-foreground">pending</span>
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium">{p.projectName}</p>
                    <p className="flex items-center gap-3 text-xs text-muted-foreground">
                      <span className="inline-flex items-center gap-1">
                        <ShieldCheck className="h-3 w-3 text-success" /> {p.governed} governed
                      </span>
                      <span className="inline-flex items-center gap-1">
                        <Sparkles className="h-3 w-3 text-info" /> {p.aiShippable} AI-shippable
                      </span>
                    </p>
                  </div>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="shrink-0"
                    onClick={(e) => {
                      e.stopPropagation();
                      onOpenReview(p.projectId, p.stream);
                    }}
                  >
                    Review <ArrowRight className="ml-1 h-3.5 w-3.5" />
                  </Button>
                </CardContent>
              </Card>
            </li>
          ))}
        </ul>
      )}

      {coverageNote && <p className="mt-3 text-xs text-muted-foreground">{coverageNote}</p>}
    </div>
  );
}
