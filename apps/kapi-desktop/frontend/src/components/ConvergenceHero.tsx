import { useCallback, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, ClipboardList, Cloud, FolderX, Loader2, PlayCircle } from "lucide-react";
import {
  Badge,
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
  RunErrorNotice,
  type RunErrorActionView,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
  parseAppError,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type {
  ConvergePlan,
  ConvergenceReport,
  ProjectServer,
  RunError,
  UpPlanScope,
} from "../types/api";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useJobFeed } from "../context/JobFeedContext";
import { ActiveModelBadge } from "./ActiveModelBadge";
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
  /** Pre-loaded run venue for Storybook/tests — skips api.getProjectServer(). */
  server?: ProjectServer;
  /** Pre-loaded last-run failure for Storybook/tests — skips api.getLastRunError(). */
  lastRunError?: RunError | null;
  /** Open an in-app settings view named by a remediation action's target. */
  onOpenSettings?: (target: string) => void;
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
  server: propServer,
  lastRunError: propLastRunError,
  onOpenSettings,
}: ConvergenceHeroProps) {
  const { hasActive, startJob, failActiveJob } = useJobFeed();
  const [planOpen, setPlanOpen] = useState(false);
  // A synchronous launch failure renders inline on the hero — the user stays
  // home instead of landing in a runner view with nothing running behind it.
  const [launchError, setLaunchError] = useState<unknown>(null);
  const [modelPromptOpen, setModelPromptOpen] = useState(false);

  // Both derivations are cheap read-only calls; react-query owns caching and the
  // "project:extracted" event invalidates them. A single query fn keeps the
  // three reads (and the files-missing derivation) atomic.
  const dataQuery = useQuery({
    queryKey: qk.convergence(tabID),
    enabled: !(propConvergence && propPlan && propServer),
    queryFn: async () => {
      const [c, p, s, e] = await Promise.allSettled([
        propConvergence ? Promise.resolve(null) : api.getConvergence(tabID),
        propPlan ? Promise.resolve(null) : api.getConvergePlan(tabID),
        propServer ? Promise.resolve(null) : api.getProjectServer(tabID),
        api.getLastRunError(),
      ]);
      // The backend answers both derivations with a single typed error when the
      // recipe is gone from disk — render the quiet reopen state.
      const filesMissing = [c, p].some(
        (r) => r.status === "rejected" && String(r.reason).includes(MISSING_FILES_MARKER),
      );
      // A derivation that FAILED is not a converged project — it is an unknown
      // one. Keeping the rejection (rather than folding it to null) is what
      // stops "compute coverage: …" from rendering as "all gates green": null
      // and "we could not tell" are different states and must stay different.
      const derivationError =
        !filesMissing && (c.status === "rejected" || p.status === "rejected")
          ? ((c.status === "rejected" ? c.reason : (p as PromiseRejectedResult).reason) as unknown)
          : null;
      return {
        convergence: c.status === "fulfilled" ? (c.value as ConvergenceReport | null) : null,
        plan: p.status === "fulfilled" ? (p.value as ConvergePlan | null) : null,
        server: s.status === "fulfilled" ? (s.value as ProjectServer | null) : null,
        lastRunError: e.status === "fulfilled" ? (e.value as RunError | null) : null,
        filesMissing,
        derivationError,
      };
    },
  });

  const convergence: ConvergenceReport | null =
    propConvergence ?? dataQuery.data?.convergence ?? null;
  const plan: ConvergePlan | null = propPlan ?? dataQuery.data?.plan ?? null;
  const server: ProjectServer | null = propServer ?? dataQuery.data?.server ?? null;
  const filesMissing = dataQuery.data?.filesMissing ?? false;
  const derivationError = dataQuery.data?.derivationError ?? null;
  const lastRunError: RunError | null = propLastRunError ?? dataQuery.data?.lastRunError ?? null;
  // "loaded" means we actually know the project's state. A settled query whose
  // derivations rejected has not told us anything, so it must not count.
  const loaded =
    (!!(propConvergence && propPlan) || dataQuery.isSuccess) &&
    derivationError == null &&
    convergence !== null &&
    plan !== null;

  // A convergence run or extraction elsewhere changed the derived state.
  useInvalidateOnEvent("project:extracted", [qk.convergence(tabID)]);

  // Remediation actions: a command copies itself inside RunErrorNotice, an
  // external page opens in the OS browser, and a settings target hands off to
  // the host so navigation stays the shell's concern.
  const onErrorAction = useCallback(
    (action: RunErrorActionView) => {
      if (action.kind === "open-url" && action.url) {
        // Same affordance the docs links use: the Wails webview hands a
        // _blank target to the OS browser.
        window.open(action.url, "_blank", "noopener,noreferrer");
        return;
      }
      if (action.kind === "open-settings" && action.target) onOpenSettings?.(action.target);
    },
    [onOpenSettings],
  );

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
  const totalsPlanned = plan?.plan?.totals;
  const missing = totalsPlanned?.missingTarget ?? 0;
  // Every unit the plan prices, not only the ones with no target file. A pass is
  // driven by the content memory, so a produced unit the corpus cannot fill is
  // work the run will do — and a hero that read only `missing` claimed a project
  // was up to date while the plan beside it quoted provider calls.
  const planned = missing + (totalsPlanned?.stale ?? 0) + (totalsPlanned?.unanswered ?? 0);
  const parked = convergence?.review?.length ?? 0;
  const storeMissing = !!plan?.storeMissing;
  const versionStale = !!plan?.versionStale;
  const gated = (convergence?.locales ?? []).filter((l) => l.gated);
  // Three states, not two: gates green, gates unmet, and *no gates at all*.
  // `[].every()` is vacuously true, so folding the third into the first let an
  // ungated recipe — or a report we never received — assert "all gates green"
  // without a single gate having been evaluated. `hasGates` keeps the claim
  // honest: no gates means no claim, not a green one.
  const hasGates = gated.length > 0;
  const gatesUnmet = hasGates && !gated.every((l) => l.shippable);
  // A failed catch-up run leaves locales unproduced no matter what the
  // file-derived tally says, so it is drift in its own right — the state cannot
  // be trusted as converged until a run completes.
  const runFailed = lastRunError != null && lastRunError.kind !== "canceled";
  const drifted = changed > 0 || planned > 0 || storeMissing || versionStale || runFailed;
  const upToDate = loaded && !drifted && !gatesUnmet;

  // The drift summary: only the non-zero pieces, joined with " · ".
  const pieces: string[] = [];
  if (runFailed) pieces.push(t("last run failed"));
  if (storeMissing) pieces.push(t("content not extracted yet"));
  else if (versionStale) pieces.push(t("store written by another kapi version"));
  if (changed > 0) pieces.push(t("{count} source file(s) changed", { count: changed }));
  if (missing > 0) pieces.push(t("{count} unit(s) missing targets", { count: missing }));
  if (parked > 0) pieces.push(t("{count} parked for review", { count: parked }));
  if (pieces.length === 0 && gatesUnmet) {
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
                {/* Only claim the gates when there are gates to claim. */}
                {hasGates ? t("Up to date · all gates green") : t("Up to date")}
              </p>
            ) : (
              <p className="text-sm font-medium" data-slot="hero-drift-summary">
                {derivationError != null
                  ? t("Could not determine project state")
                  : loaded
                    ? pieces.join(" · ") || t("Pending work")
                    : t("Deriving project state…")}
              </p>
            )}
            <p className="text-xs text-muted-foreground">
              {upToDate
                ? hasGates
                  ? t("Every unit has a committed target and every gated scope ships.")
                  : t("Every unit has a committed target. This recipe declares no ship gates.")
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
            {/* Which model this run will use — visible where the run is
                launched, not only in Settings. */}
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
              {server?.connected && <VenueBadge server={server} />}
              <ActiveModelBadge tabID={tabID} />
            </div>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPlanOpen(true)}
            disabled={!loaded}
            data-slot="hero-plan"
            aria-label={t("Preview the catch-up plan")}
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

      {/* A failed catch-up run stays visible on the home surface until the next
          run clears it — prominently, and with its remedy attached, rather than
          being represented only as an absence in the coverage numbers. */}
      {launchError == null && runFailed && lastRunError != null && (
        <div data-slot="hero-run-error" className="mt-3">
          <RunErrorNotice
            error={lastRunError}
            context={t("Last run failed")}
            detailsLabel={t("Details")}
            copiedLabel={t("Copied")}
            onAction={onErrorAction}
          />
        </div>
      )}

      {/* A derivation that failed is reported as such — never as convergence. */}
      {derivationError != null && (
        <div data-slot="hero-derivation-error" className="mt-3">
          <ErrorNotice
            error={derivationError}
            title={t("Could not determine project state")}
            hint={t("The numbers below may be stale. Re-open the project or run again.")}
            variant="panel"
            detailsLabel={t("Details")}
          />
        </div>
      )}

      <ConvergePlanDialog
        open={planOpen}
        onOpenChange={setPlanOpen}
        plan={plan}
        server={server}
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

/**
 * VenueBadge discloses where `kapi up` canonically runs. A project connected to
 * a Bowrain server runs its convergence on the server; the desktop currently
 * drives the local engine, so the badge is honest about that split rather than
 * implying a remote run happened.
 */
export function VenueBadge({ server }: { server: ProjectServer }) {
  if (!server.connected) return null;
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="secondary" className="gap-1 font-normal" data-slot="venue-badge">
            <Cloud size={11} />
            {server.host
              ? t("Connected to Bowrain · {host}", { host: server.host })
              : t("Connected to Bowrain")}
          </Badge>
        </TooltipTrigger>
        <TooltipContent className="max-w-xs">
          {t(
            "This project is connected to Bowrain, so kapi up runs on the server. Bringing it up to date here runs the same engine locally.",
          )}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

export interface ConvergePlanDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plan: ConvergePlan | null;
  /** The run venue — shows a "Connected to Bowrain" note when server-backed. */
  server?: ProjectServer | null;
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
 * (collection, locale): units missing a target, exact content-memory leverage, remaining
 * AI work — with the total token estimate and its disclosed heuristic. Nothing
 * runs until the user confirms.
 */
export function ConvergePlanDialog({
  open,
  onOpenChange,
  plan,
  server,
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
          <DialogTitle>{t("Catch-up plan")}</DialogTitle>
          <DialogDescription>
            {t("A dry run of the pending work. Nothing is written and no provider is called.")}
          </DialogDescription>
        </DialogHeader>

        {server?.connected && (
          <p
            className="flex items-center gap-1.5 text-xs text-muted-foreground"
            data-slot="plan-venue"
          >
            <Cloud size={12} className="shrink-0" />
            {server.host
              ? t(
                  "Connected to Bowrain ({host}): kapi up runs on the server; bringing up to date here runs the same engine locally.",
                  { host: server.host },
                )
              : t(
                  "Connected to Bowrain: kapi up runs on the server; bringing up to date here runs the same engine locally.",
                )}
          </p>
        )}

        {plan?.storeMissing && (
          <p className="text-xs text-muted-foreground">
            {t("The project has not been extracted yet. The run extracts it first.")}
          </p>
        )}
        {changed > 0 && (
          <p className="text-xs text-muted-foreground">
            {t(
              "{count} source file(s) changed since the last extract. The run re-extracts them first.",
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
          <Table className="text-xs" data-slot="plan-table">
            <TableHeader>
              <TableRow className="text-muted-foreground hover:bg-transparent">
                <TableHead className="h-auto py-1.5 pr-2 text-muted-foreground">
                  {t("Scope")}
                </TableHead>
                <TableHead className="h-auto px-2 py-1.5 text-right text-muted-foreground">
                  {t("Missing")}
                </TableHead>
                <TableHead className="h-auto px-2 py-1.5 text-right text-muted-foreground">
                  {t("Memory exact")}
                </TableHead>
                <TableHead className="h-auto px-2 py-1.5 text-right text-muted-foreground">
                  {t("Stored drafts")}
                </TableHead>
                <TableHead className="h-auto px-2 py-1.5 text-right text-muted-foreground">
                  {t("AI work")}
                </TableHead>
                <TableHead className="h-auto py-1.5 pl-2 text-right text-muted-foreground">
                  {t("~tokens")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {scopes.map((s, i) => (
                <TableRow
                  key={`${s.locale}-${s.collection}-${i}`}
                  className="border-border/50 hover:bg-transparent"
                >
                  <TableCell className="py-1.5 pr-2">{scopeName(s)}</TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {s.missingTarget}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">{s.tmExact}</TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {s.drafts ?? 0}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {s.aiRemaining}
                  </TableCell>
                  <TableCell className="py-1.5 pl-2 text-right tabular-nums">
                    {s.tokenEstimate}
                  </TableCell>
                </TableRow>
              ))}
              {totals && (
                <TableRow className="font-medium hover:bg-transparent" data-slot="plan-totals">
                  <TableCell className="py-1.5 pr-2">{t("Total")}</TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {totals.missingTarget}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {totals.tmExact}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {totals.drafts ?? 0}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {totals.aiRemaining}
                  </TableCell>
                  <TableCell className="py-1.5 pl-2 text-right tabular-nums">
                    {totals.tokenEstimate}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        )}

        {subscription && (
          <p className="text-xs font-medium" data-slot="plan-subscription">
            {t("AI work runs on your Claude subscription, with no per-token API cost.")}
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
