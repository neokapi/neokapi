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
  coordinatesOpen,
  planFromServer,
  type CoordinateChange,
  planProducts,
  planRuleLabel,
  type PlanLevel,
  type PlanRule,
  type ReviewInvolvement,
  type ReviewPlan,
} from "@neokapi/bowrain-client";

export interface ReviewPlanPanelProps {
  ws: string;
  projectId: string;
  stream: string;
}

/** What each involvement leaves to a person and to the checks. */
function InvolvementDetail({ involvement }: { involvement: ReviewInvolvement }) {
  switch (involvement) {
    case "interfaces":
      return <>As Context, and you read every interface page.</>;
    case "every":
      return <>A person establishes every translation. Nothing is settled by the checks.</>;
    default:
      return (
        <>
          You decide the coordinates, the key language and the exceptions, and read a sample of
          pages. Content that clears every check is settled by the checks.
        </>
      );
  }
}

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

/** The rules behind the most findings in one place: the source or a language. */
function RuleList({ testId, title, rules }: { testId: string; title: string; rules: PlanRule[] }) {
  if (rules.length === 0) return null;
  return (
    <div data-testid={testId} className="pt-1">
      <p className="text-xs font-semibold uppercase">{title}</p>
      <ul className="space-y-0.5">
        {rules.map((r) => (
          <li key={r.key} className="flex items-baseline gap-2">
            <span className="text-foreground min-w-0 flex-1 truncate" title={r.rule}>
              {planRuleLabel(r)}
            </span>
            <span className="shrink-0 tabular-nums">
              {r.count} {r.count === 1 ? "finding" : "findings"}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function placeOf(coords: Record<string, string> | undefined): string {
  return [coords?.product, coords?.channel].filter(Boolean).join(" / ");
}

/** One change against the confirmed coordinates, as a sentence. */
function ChangeText({ change: c }: { change: CoordinateChange }) {
  const where = placeOf(c.coordinates);
  switch (c.kind) {
    case "point_added":
      return (
        <>
          New: {c.collection} at {where}
        </>
      );
    case "point_removed":
      return (
        <>
          Removed: {c.collection} at {where}
        </>
      );
    case "point_moved":
      return (
        <>
          Moved: {c.collection} from {placeOf(c.previousCoordinates)} to {where}
        </>
      );
    case "voice_changed":
      return c.voice ? (
        <>
          Voice: {c.collection} now uses {c.voice}
        </>
      ) : (
        <>Voice: {c.collection} no longer has a voice</>
      );
    case "source_only_changed":
      return c.sourceOnly ? (
        <>{c.collection} is now source only</>
      ) : (
        <>{c.collection} now receives translations</>
      );
    case "language_added":
      return <>Language added: {c.locale}</>;
    case "language_removed":
      return <>Language removed: {c.locale}</>;
  }
}

/** What changed since the team last confirmed the coordinates. */
function CoordinateChanges({ by, changes }: { by?: string; changes: CoordinateChange[] }) {
  return (
    <div data-testid="plan-coordinate-changes" className="pt-1">
      <p className="text-xs font-semibold uppercase">
        {by ? <>Since {by} confirmed</> : <>Since the last confirmation</>}
      </p>
      {changes.length === 0 ? (
        <p>Nothing changed since the last confirmation.</p>
      ) : (
        <ul className="space-y-0.5">
          {changes.map((c, i) => (
            <li key={`${c.kind}-${c.collection ?? c.locale}-${i}`} className="text-foreground">
              <ChangeText change={c} />
            </li>
          ))}
        </ul>
      )}
    </div>
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
  onSettle,
  onScan,
  busy,
}: {
  plan: ReviewPlan;
  ws: string;
  projectId: string;
  stream: string;
  onConfirmCoordinates?: () => void;
  onInvolvement?: (v: ReviewInvolvement) => void;
  onSettle?: () => void;
  onScan?: () => void;
  busy?: boolean;
}) {
  const products = planProducts(plan.coordinates.points);
  const untranslated = Object.entries(plan.keyLanguage.untranslated).filter(([, n]) => n > 0);
  const failing = plan.languages.reduce((n, l) => n + l.failing, 0);
  const pages = plan.languages.reduce((n, l) => n + l.pages, 0);
  const pagesTotal = plan.languages.reduce((n, l) => n + l.pagesTotal, 0);
  const scanning =
    plan.keyLanguage.scan?.status === "queued" || plan.keyLanguage.scan?.status === "processing";
  const settleNow = plan.policy.settle === "first_pass" || plan.policy.settle === "policy";
  const settleable = plan.languages.reduce((n, l) => n + l.settleable, 0);
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
            <option value="context">Context</option>
            <option value="interfaces">Interfaces</option>
            <option value="every">Every translation</option>
          </select>
        </label>
      </div>
      <p className="text-muted-foreground text-sm">
        Decide what generates the text before you read the text.{" "}
        <InvolvementDetail involvement={plan.involvement} />
      </p>
      <ol className="space-y-2">
        <Step
          n={1}
          level="coordinates"
          next={plan.next}
          done={!coordinatesOpen(plan.coordinates)}
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
          {plan.coordinates.confirmed && (
            <CoordinateChanges
              by={plan.coordinates.confirmedByName || plan.coordinates.confirmedBy}
              changes={plan.coordinates.changes}
            />
          )}
          {coordinatesOpen(plan.coordinates) && onConfirmCoordinates && (
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
          {scanning && (
            <p>Scanning the project for terms, {plan.keyLanguage.scan?.progress ?? 0}% done</p>
          )}
          {plan.keyLanguage.scan?.status === "completed" && (
            <p>The last scan finished. Its terms are in the context deck.</p>
          )}
          {plan.keyLanguage.scan?.status === "failed" && (
            <p>The last scan failed. You can start it again.</p>
          )}
          <div className="flex flex-wrap items-center gap-3">
            <Link
              to="/$workspace/p/$projectId/s/$stream/rules"
              params={params}
              className="text-primary"
            >
              Decide the key language
            </Link>
            {onScan && !scanning && (
              <Button size="sm" variant="outline" disabled={busy} onClick={onScan}>
                Scan the project for terms
              </Button>
            )}
          </div>
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
          {(plan.exceptions.byRule.source.length > 0 ||
            Object.values(plan.exceptions.byRule.targets).some((rs) => rs.length > 0)) && (
            <p>
              Most findings break one rule many times. Change the rule and its findings clear, or
              fix the source and its translations are drafted again.
            </p>
          )}
          <RuleList
            testId="plan-rules-source"
            title="Source, by rule"
            rules={plan.exceptions.byRule.source}
          />
          {plan.languages.map((l) => (
            <RuleList
              key={l.locale}
              testId={`plan-rules-${l.locale}`}
              title={`${l.locale}, by rule`}
              rules={plan.exceptions.byRule.targets[l.locale] ?? []}
            />
          ))}
        </Step>
        <Step
          n={4}
          level="interfaces"
          next={plan.next}
          done={pages === 0}
          title="Interfaces in context"
        >
          {plan.involvement === "context" ? (
            <p>
              Read {pages} of {pagesTotal} pages: one in ten from each channel, and every page with
              a failing check.
            </p>
          ) : (
            <p>{pages} pages with a translation waiting. Read every page, source first.</p>
          )}
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
          {plan.policy.settle === "none" ? (
            <p>Every translation is read by a person, so nothing is settled here.</p>
          ) : (
            <>
              <p>{settleable} translations clear every check.</p>
              {plan.policy.settle === "held" && (
                <p>
                  Held until the key language is decided: {plan.policy.keyLanguageOpen} decisions
                  are still open.
                </p>
              )}
              {plan.policy.settle === "first_pass" && (
                <p>
                  First pass: this settles on the checks alone. Later passes wait until the key
                  language is decided.
                </p>
              )}
              {plan.policy.settle === "policy" && (
                <p>
                  Each is established as your decision, recorded in the ledger, and reopened if
                  anyone rejects it later.
                </p>
              )}
              {settleNow && settleable > 0 && onSettle && (
                <Button size="sm" variant="outline" disabled={busy} onClick={onSettle}>
                  Establish the {settleable} translations
                </Button>
              )}
            </>
          )}
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
  const settle = useMutation({
    mutationFn: () => api.settleReviewPlan!(ws, projectId, stream),
    onSuccess: refresh,
  });
  const scan = useMutation({
    mutationFn: () => api.startReviewPlanScan!(ws, projectId),
    onSuccess: refresh,
  });
  if (!plan.data) return null;
  return (
    <ReviewPlanSteps
      plan={plan.data}
      ws={ws}
      projectId={projectId}
      stream={stream}
      busy={confirm.isPending || involve.isPending || settle.isPending || scan.isPending}
      onConfirmCoordinates={api.confirmReviewCoordinates ? () => confirm.mutate() : undefined}
      onInvolvement={api.setReviewInvolvement ? (v) => involve.mutate(v) : undefined}
      onSettle={api.settleReviewPlan ? () => settle.mutate() : undefined}
      onScan={api.startReviewPlanScan ? () => scan.mutate() : undefined}
    />
  );
}
