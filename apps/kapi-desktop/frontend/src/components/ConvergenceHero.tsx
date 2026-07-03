import { useCallback, useEffect, useState } from "react";
import {
  CheckCircle2,
  ClipboardList,
  FolderX,
  Loader2,
  PlayCircle,
} from "lucide-react";
import {
  Button,
  Card,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  ErrorNotice,
  LocalePill,
  parseAppError,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { ConvergePlan, ConvergenceReport, UpPlanScope } from "../types/api";
import { api } from "../hooks/useApi";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { useJobFeed } from "../context/JobFeedContext";
import { AIModelPromptDialog } from "./AIModelPromptDialog";

/** The backend's typed marker for a tab whose recipe vanished from disk. */
const MISSING_FILES_MARKER = "missing or moved";

export interface ConvergenceHeroProps {
  tabID: string;
  /**
   * Navigate to the runner's passes view. Called only AFTER the run launched
   * successfully — a synchronous launch error stays on the hero (inline)
   * instead of opening a dead runner.
   */
  onBringUpToDate?: () => void;
  /** Pre-loaded report for Storybook/tests — skips api.getConvergence(). */
  convergence?: ConvergenceReport;
  /** Pre-loaded plan for Storybook/tests — skips api.getConvergePlan(). */
  plan?: ConvergePlan;
}

/**
 * The convergence hero strip at the top of the project home (issue #1078 C4):
 * a drift summary derived from the pre-flight plan + convergence report
 * ("N source files changed · M units missing targets · K parked for review"),
 * a primary "Bring up to date" that runs the shared `kapi up` engine, and a
 * "Plan…" pre-flight dialog. When everything is converged and the gates are
 * green it renders the calm state with the button disabled. Both derivations
 * are cheap, read-only calls made on load and after a run/extract.
 */
export function ConvergenceHero({
  tabID,
  onBringUpToDate,
  convergence: propConvergence,
  plan: propPlan,
}: ConvergenceHeroProps) {
  const { hasActive, startJob, failActiveJob } = useJobFeed();
  const [convergence, setConvergence] = useState<ConvergenceReport | null>(propConvergence ?? null);
  const [plan, setPlan] = useState<ConvergePlan | null>(propPlan ?? null);
  const [planOpen, setPlanOpen] = useState(false);
  const [loaded, setLoaded] = useState(!!(propConvergence && propPlan));
  // A synchronous launch failure renders inline on the hero — the user stays
  // home instead of landing in a runner view with nothing running behind it.
  const [launchError, setLaunchError] = useState<unknown>(null);
  // The tab's files vanished from disk (moved/deleted directory): a quiet
  // terminal state, not an error banner per poll.
  const [filesMissing, setFilesMissing] = useState(false);
  const [modelPromptOpen, setModelPromptOpen] = useState(false);

  const refresh = useCallback(() => {
    if (propConvergence && propPlan) return;
    void Promise.allSettled([
      propConvergence ? Promise.resolve(null) : api.getConvergence(tabID),
      propPlan ? Promise.resolve(null) : api.getConvergePlan(tabID),
    ]).then(([c, p]) => {
      if (!propConvergence && c.status === "fulfilled" && c.value) {
        setConvergence(c.value as ConvergenceReport);
      }
      if (!propPlan && p.status === "fulfilled" && p.value) setPlan(p.value as ConvergePlan);
      // The backend answers both derivations with a single typed error when
      // the recipe is gone from disk — render the quiet reopen state.
      setFilesMissing(
        [c, p].some(
          (r) => r.status === "rejected" && String(r.reason).includes(MISSING_FILES_MARKER),
        ),
      );
      setLoaded(true);
    });
  }, [tabID, propConvergence, propPlan]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // A convergence run or extraction elsewhere changed the derived state.
  useWailsEvent("project:extracted", () => refresh());

  // doLaunch starts the run through the shared `kapi up` engine and navigates
  // to the runner only once the launch call succeeded. A rejected launch
  // settles the pre-created job and surfaces the message inline right here.
  const doLaunch = useCallback(async () => {
    startJob("up");
    try {
      await api.bringUpToDate(tabID);
    } catch (err) {
      // Keep the friendly line in the job feed; keep the full error here for
      // the inline notice (the user stays on home — no navigation on failure).
      failActiveJob(parseAppError(err).title);
      setLaunchError(err);
      return;
    }
    onBringUpToDate?.();
  }, [tabID, startJob, failActiveJob, onBringUpToDate]);

  // ── Derived standing ──────────────────────────────────────────────────────
  const changed = (plan?.changedFiles ?? 0) + (plan?.removedFiles ?? 0);
  const missing = plan?.plan?.totals?.missingTarget ?? 0;
  const parked = convergence?.review?.length ?? 0;
  const storeMissing = !!plan?.storeMissing;
  const versionStale = !!plan?.versionStale;
  const gated = (convergence?.locales ?? []).filter((l) => l.gated);
  const gatesGreen = gated.every((l) => l.shippable);
  const drifted = changed > 0 || missing > 0 || storeMissing || versionStale;
  const upToDate = loaded && !drifted && gatesGreen;

  // The drift summary: only the non-zero pieces, joined with " · ".
  const pieces: string[] = [];
  if (storeMissing) pieces.push(t("content not extracted yet"));
  else if (versionStale) pieces.push(t("store written by another kapi version"));
  if (changed > 0) pieces.push(t("{count} source file(s) changed", { count: changed }));
  if (missing > 0) pieces.push(t("{count} unit(s) missing targets", { count: missing }));
  if (parked > 0) pieces.push(t("{count} parked for review", { count: parked }));
  if (pieces.length === 0 && !gatesGreen && loaded) {
    pieces.push(t("ship gates not met yet"));
  }

  // launch: model pre-flight first (the built-in default flow's translate step
  // is provider-backed), then the actual run.
  const launch = () => {
    setPlanOpen(false);
    setLaunchError(null);
    if (!onBringUpToDate) return;
    void (async () => {
      try {
        if (await api.aiNeedsModelChoice(tabID, "")) {
          setModelPromptOpen(true);
          return;
        }
      } catch {
        // Pre-flight unavailable — let the launch surface any real error.
      }
      await doLaunch();
    })();
  };

  if (filesMissing) {
    return (
      <Card className="mb-6 p-4" data-slot="convergence-hero">
        <div className="flex items-center gap-2.5" data-slot="hero-files-missing">
          <FolderX size={18} className="shrink-0 text-muted-foreground" />
          <div className="min-w-0">
            <p className="text-sm font-medium">{t("Project files are missing or moved")}</p>
            <p className="text-xs text-muted-foreground">
              {t(
                "The project directory is no longer where it was. Reopen the project to continue.",
              )}
            </p>
          </div>
        </div>
      </Card>
    );
  }

  return (
    <Card className="mb-6 p-4" data-slot="convergence-hero">
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex min-w-0 flex-1 items-center gap-2.5">
          {upToDate ? (
            <CheckCircle2 size={18} className="shrink-0 text-green-500" />
          ) : (
            <PlayCircle size={18} className="shrink-0 text-primary" />
          )}
          <div className="min-w-0">
            {upToDate ? (
              <p className="text-sm font-medium" data-slot="hero-up-to-date">
                {t("Up to date · all gates green")}
              </p>
            ) : (
              <p className="text-sm font-medium" data-slot="hero-drift-summary">
                {loaded ? pieces.join(" · ") || t("Pending work") : t("Deriving project state…")}
              </p>
            )}
            <p className="text-xs text-muted-foreground">
              {upToDate
                ? t("Every unit has a committed target and every gated scope ships.")
                : t(
                    "Bring up to date extracts changed sources, runs the default flow to the ship gates, and parks what needs a human.",
                  )}
              {!upToDate && plan?.plan?.subscription && (
                <span data-slot="hero-subscription">
                  {" "}
                  {t("AI runs on your Claude subscription.")}
                </span>
              )}
            </p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPlanOpen(true)}
            disabled={!loaded}
            data-slot="hero-plan"
            aria-label={t("Preview the convergence plan")}
          >
            <ClipboardList size={13} />
            {t("Plan…")}
          </Button>
          <Button
            size="sm"
            onClick={launch}
            disabled={upToDate || hasActive || !onBringUpToDate}
            data-slot="hero-bring-up-to-date"
            aria-label={t("Bring the project up to date")}
          >
            {hasActive ? <Loader2 size={13} className="animate-spin" /> : <PlayCircle size={13} />}
            {t("Bring up to date")}
          </Button>
        </div>
      </div>

      {launchError != null && (
        <div data-slot="hero-launch-error" className="mt-3">
          <ErrorNotice error={launchError} variant="panel" detailsLabel={t("Details")} />
        </div>
      )}

      <ConvergePlanDialog
        open={planOpen}
        onOpenChange={setPlanOpen}
        plan={plan}
        onConfirm={onBringUpToDate ? launch : undefined}
        confirmDisabled={hasActive || upToDate}
      />

      <AIModelPromptDialog
        open={modelPromptOpen}
        onResolved={() => {
          setModelPromptOpen(false);
          void doLaunch();
        }}
        onCancel={() => setModelPromptOpen(false)}
      />
    </Card>
  );
}

export interface ConvergePlanDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plan: ConvergePlan | null;
  /** Confirm the pre-flight and launch the run. Hidden when absent. */
  onConfirm?: () => void;
  confirmDisabled?: boolean;
}

/** One row label: locale pill + optional collection. */
function scopeName(s: UpPlanScope) {
  return (
    <span className="flex items-center gap-1.5">
      {s.locale ? <LocalePill locale={s.locale} /> : null}
      {s.collection && (
        <span className="truncate text-muted-foreground" translate="no">
          {s.collection}
        </span>
      )}
    </span>
  );
}

/**
 * The pre-flight plan dialog: the dry-run work `kapi up --plan` derives — per
 * (collection, locale): units missing a target, exact TM leverage, remaining
 * AI work — with the total token estimate and its disclosed heuristic. Nothing
 * runs until the user confirms.
 */
export function ConvergePlanDialog({
  open,
  onOpenChange,
  plan,
  onConfirm,
  confirmDisabled,
}: ConvergePlanDialogProps) {
  const scopes = plan?.plan?.scopes ?? [];
  const totals = plan?.plan?.totals;
  const changed = (plan?.changedFiles ?? 0) + (plan?.removedFiles ?? 0);
  const subscription = !!plan?.plan?.subscription;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg" data-slot="converge-plan-dialog">
        <DialogHeader>
          <DialogTitle>{t("Convergence plan")}</DialogTitle>
          <DialogDescription>
            {t("A dry run of the pending work — nothing is written and no provider is called.")}
          </DialogDescription>
        </DialogHeader>

        {plan?.storeMissing && (
          <p className="text-xs text-muted-foreground">
            {t("The project has not been extracted yet — the run extracts it first.")}
          </p>
        )}
        {changed > 0 && (
          <p className="text-xs text-muted-foreground">
            {t(
              "{count} source file(s) changed since the last extract — the run re-extracts them first.",
              {
                count: changed,
              },
            )}
          </p>
        )}

        {scopes.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground" data-slot="plan-empty">
            {t("Nothing to do: every unit has a committed target.")}
          </p>
        ) : (
          <table className="w-full text-xs" data-slot="plan-table">
            <thead>
              <tr className="border-b border-border text-left text-muted-foreground">
                <th className="py-1.5 pr-2 font-medium">{t("Scope")}</th>
                <th className="px-2 py-1.5 text-right font-medium">{t("Missing")}</th>
                <th className="px-2 py-1.5 text-right font-medium">{t("TM exact")}</th>
                <th className="px-2 py-1.5 text-right font-medium">{t("AI work")}</th>
                <th className="py-1.5 pl-2 text-right font-medium">{t("~tokens")}</th>
              </tr>
            </thead>
            <tbody>
              {scopes.map((s, i) => (
                <tr key={`${s.locale}-${s.collection}-${i}`} className="border-b border-border/50">
                  <td className="py-1.5 pr-2">{scopeName(s)}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums">{s.missingTarget}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums">{s.tmExact}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums">{s.aiRemaining}</td>
                  <td className="py-1.5 pl-2 text-right tabular-nums">{s.tokenEstimate}</td>
                </tr>
              ))}
              {totals && (
                <tr className="font-medium" data-slot="plan-totals">
                  <td className="py-1.5 pr-2">{t("Total")}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums">{totals.missingTarget}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums">{totals.tmExact}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums">{totals.aiRemaining}</td>
                  <td className="py-1.5 pl-2 text-right tabular-nums">{totals.tokenEstimate}</td>
                </tr>
              )}
            </tbody>
          </table>
        )}

        {subscription && (
          <p className="text-xs font-medium" data-slot="plan-subscription">
            {t("AI work runs on your Claude subscription — no per-token API cost.")}
          </p>
        )}

        {plan?.plan?.note && (
          <p className="text-[11px] text-muted-foreground" data-slot="plan-note">
            {plan.plan.note}
          </p>
        )}

        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            {t("Cancel")}
          </Button>
          {onConfirm && (
            <Button
              size="sm"
              onClick={onConfirm}
              disabled={confirmDisabled}
              data-slot="plan-confirm"
            >
              <PlayCircle size={13} />
              {t("Bring up to date")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
