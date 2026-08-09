import { Card, CardContent, Button, cn } from "@neokapi/ui-primitives";
import { Eye, CircleCheck, Sparkles, ShieldCheck, ArrowRight, Lock } from "../icons";

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

/** One open review task assigned to (or claimable by) the current user. */
export interface ReviewInboxTask {
  id: string;
  title: string;
  /** Task type — "review_terms" is a governed change-set review. */
  type: string;
  /** Set on change-set review tasks; links straight to the change-set. */
  changesetId?: string;
  projectId?: string;
  stream?: string;
}

export interface ReviewInboxProps {
  /** Projects with pending review, most-pending first. */
  projects: ReviewInboxProject[];
  /**
   * The user's open review tasks — the same set the dashboard counts as
   * "open review task(s) for you". The dashboard's card links here, so a
   * task that is counted there must be visible here.
   */
  tasks?: ReviewInboxTask[];
  /** Open a project's review session. */
  onOpenReview: (projectId: string, stream: string) => void;
  /** Open what a task is waiting on (change-set page or review session). */
  onOpenTask?: (task: ReviewInboxTask) => void;
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
export function ReviewInbox({
  projects,
  tasks = [],
  onOpenReview,
  onOpenTask,
  loading,
  coverageNote,
}: ReviewInboxProps) {
  const withPending = projects.filter((p) => p.pending > 0);
  const empty = withPending.length === 0 && tasks.length === 0;

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
      ) : empty ? (
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
        <>
          {tasks.length > 0 && (
            <section className="mb-4 space-y-2" data-testid="review-inbox-tasks">
              <h2 className="text-sm font-medium text-muted-foreground">Waiting on you</h2>
              <ul className="space-y-2">
                {tasks.map((t) => (
                  <li key={t.id}>
                    <Card
                      className={cn("cursor-pointer transition-colors hover:border-primary/40")}
                      onClick={() => onOpenTask?.(t)}
                      data-testid={`review-inbox-task-${t.id}`}
                    >
                      <CardContent className="flex items-center gap-3 py-3">
                        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/10 text-primary">
                          <Lock className="h-4 w-4" />
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="truncate font-medium">{t.title}</p>
                          <p className="text-xs text-muted-foreground">
                            {t.type === "review_terms"
                              ? "A governed change-set needs your decision"
                              : "A review task assigned to you"}
                          </p>
                        </div>
                        <ArrowRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                      </CardContent>
                    </Card>
                  </li>
                ))}
              </ul>
            </section>
          )}
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
        </>
      )}

      {coverageNote && <p className="mt-3 text-xs text-muted-foreground">{coverageNote}</p>}
    </div>
  );
}
