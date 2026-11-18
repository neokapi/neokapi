// The review plan on a project's Overview: what the project asks of its
// reviewers, ordered by the reach of each decision. The coordinates and the key
// language generate the text, so they come first; then the exceptions, the
// interfaces read in context, and what the involvement leaves to the checks.
// The server counts every level (GET /:ws/:id/review/plan); each step links to
// the page that decides it.

import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, useApi } from "@neokapi/ui";
import {
  planFromServer,
  planProducts,
  type PlanLevel,
  type ReviewInvolvement,
  type ReviewPlan,
} from "@neokapi/bowrain-client";

export interface ReviewPlanPanelProps {
  ws: string;
  projectId: string;
  stream: string;
}

const INVOLVEMENTS: { value: ReviewInvolvement; label: string; detail: string }[] = [
  {
    value: "context",
    label: "Context",
    detail:
      "You decide the coordinates, the key language and the exceptions, and read a sample of pages. Content that clears every check is settled by the checks.",
  },
  {
    value: "interfaces",
    label: "Interfaces",
    detail: "As Context, and you read every interface page.",
  },
  {
    value: "every",
    label: "Every translation",
    detail: "A person establishes every translation. Nothing is settled by the checks.",
  },
];

function reviewPlanQueryKey(ws: string, projectId: string, stream: string) {
  return ["review-plan", ws, projectId, stream] as const;
}

function Step({
  n,
  level,
  next,
  done,
  title,
  children,
}: {
  n: number;
  level: PlanLevel;
  next: PlanLevel | null;
  done: boolean;
  title: string;
  children: React.ReactNode;
}) {
  const isNext = next === level;
  return (
    <li
      data-testid={`plan-${level}`}
      data-next={isNext || undefined}
      className={`flex gap-3 rounded-lg border p-3 ${isNext ? "border-primary bg-primary/5" : "border-border"}`}
    >
      <span
        className={`flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold ${
          done
            ? "bg-success text-success-foreground"
            : isNext
              ? "bg-primary text-primary-foreground"
              : "bg-muted text-muted-foreground"
        }`}
      >
        {done ? "✓" : n}
      </span>
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex items-baseline gap-2">
          <span className="font-medium">{title}</span>
          {isNext && <span className="text-primary text-xs font-semibold uppercase">Next</span>}
        </div>
        <div className="text-muted-foreground space-y-0.5 text-sm">{children}</div>
      </div>
    </li>
  );
}

/** The plan's steps, drawn from the server's answer. */
export function ReviewPlanSteps({
  plan,
  ws,
  projectId,
  stream,
  onConfirmCoordinates,
  onInvolvement,
  busy,
}: {
  plan: ReviewPlan;
  ws: string;
  projectId: string;
  stream: string;
  onConfirmCoordinates?: () => void;
  onInvolvement?: (v: ReviewInvolvement) => void;
  busy?: boolean;
}) {
  const products = planProducts(plan.coordinates.points);
  const untranslated = Object.entries(plan.keyLanguage.untranslated).filter(([, n]) => n > 0);
  const failing = plan.languages.reduce((n, l) => n + l.failing, 0);
  const pages = plan.languages.reduce((n, l) => n + l.pages, 0);
  const settleable = plan.languages.reduce((n, l) => n + l.settleable, 0);
  const involvement = INVOLVEMENTS.find((i) => i.value === plan.involvement) ?? INVOLVEMENTS[0];
  const params = { workspace: ws, projectId, stream };

  return (
    <section data-testid="review-plan" className="space-y-3 rounded-xl border p-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="text-base font-semibold">Review plan</h2>
        <label className="text-muted-foreground flex items-center gap-2 text-sm">
          Involvement
          <select
            data-testid="plan-involvement"
            className="bg-background rounded-md border px-2 py-1 text-sm"
            value={plan.involvement}
            disabled={!onInvolvement || busy}
            onChange={(e) => onInvolvement?.(e.target.value as ReviewInvolvement)}
          >
            {INVOLVEMENTS.map((i) => (
              <option key={i.value} value={i.value}>
                {i.label}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p className="text-muted-foreground text-sm">
        Decide what generates the text before you read the text. {involvement.detail}
      </p>
      <ol className="space-y-2">
        <Step
          n={1}
          level="coordinates"
          next={plan.next}
          done={plan.coordinates.confirmed}
          title="Coordinates"
        >
          <p>
            {plan.coordinates.sourceLanguage} into{" "}
            {plan.coordinates.targetLanguages.join(", ") || "no target language yet"}
          </p>
          {products.map((p) => (
            <p key={p.product}>
              {p.product}: {p.channels.join(", ")}
            </p>
          ))}
          {!plan.coordinates.confirmed && onConfirmCoordinates && (
            <Button size="sm" variant="outline" disabled={busy} onClick={onConfirmCoordinates}>
              Confirm coordinates
            </Button>
          )}
        </Step>
        <Step
          n={2}
          level="key_language"
          next={plan.next}
          done={plan.keyLanguage.waiting === 0 && untranslated.length === 0}
          title="Key language"
        >
          <p>
            {plan.keyLanguage.waiting} terms and voice rules to decide · {plan.keyLanguage.concepts}{" "}
            terms in the source language
          </p>
          {untranslated.map(([locale, n]) => (
            <p key={locale}>
              {n} terms have no {locale} term yet
            </p>
          ))}
          <p>
            <Link
              to="/$workspace/p/$projectId/s/$stream/rules"
              params={params}
              className="text-primary"
            >
              Decide the key language
            </Link>
          </p>
        </Step>
        <Step
          n={3}
          level="exceptions"
          next={plan.next}
          done={(plan.exceptions.sourceFindings ?? 0) === 0 && failing === 0}
          title="Exceptions"
        >
          <p>
            {plan.exceptions.sourceFindings === null
              ? "The source has not been checked yet"
              : `${plan.exceptions.sourceFindings} findings in the source`}{" "}
            · {failing} translations fail a check
          </p>
        </Step>
        <Step
          n={4}
          level="interfaces"
          next={plan.next}
          done={pages === 0}
          title="Interfaces in context"
        >
          <p>
            {pages} pages with a translation waiting.{" "}
            {plan.involvement === "context"
              ? "Read a sample to confirm the decisions above read right."
              : "Read every page, source first."}
          </p>
          {pages > 0 && (
            <p>
              <Link
                to="/$workspace/p/$projectId/s/$stream/review"
                params={params}
                className="text-primary"
              >
                Read the pages
              </Link>
            </p>
          )}
        </Step>
        <Step
          n={5}
          level="policy"
          next={plan.next}
          done={settleable === 0}
          title="Settled by the checks"
        >
          <p>
            {plan.involvement === "every"
              ? "Every translation is read by a person, so nothing is settled here."
              : `${settleable} translations clear every check. Each is established as your decision, recorded in the ledger, and reopened if anyone rejects it later.`}
          </p>
        </Step>
      </ol>
    </section>
  );
}

/** The review plan panel, read and written through the server. */
export function ReviewPlanPanel({ ws, projectId, stream }: ReviewPlanPanelProps) {
  const api = useApi();
  const qc = useQueryClient();
  const key = reviewPlanQueryKey(ws, projectId, stream);
  const plan = useQuery({
    queryKey: key,
    queryFn: async () => planFromServer(await api.getReviewPlan!(ws, projectId, stream)),
    enabled: !!api.getReviewPlan,
  });
  const refresh = () => void qc.invalidateQueries({ queryKey: key });
  const confirm = useMutation({
    mutationFn: () => api.confirmReviewCoordinates!(ws, projectId),
    onSuccess: refresh,
  });
  const involve = useMutation({
    mutationFn: (v: ReviewInvolvement) => api.setReviewInvolvement!(ws, projectId, v),
    onSuccess: refresh,
  });
  if (!plan.data) return null;
  return (
    <ReviewPlanSteps
      plan={plan.data}
      ws={ws}
      projectId={projectId}
      stream={stream}
      busy={confirm.isPending || involve.isPending}
      onConfirmCoordinates={api.confirmReviewCoordinates ? () => confirm.mutate() : undefined}
      onInvolvement={api.setReviewInvolvement ? (v) => involve.mutate(v) : undefined}
    />
  );
}
