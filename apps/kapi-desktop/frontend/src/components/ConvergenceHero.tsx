import { useCallback, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  CheckCircle2,
  ClipboardList,
  Cloud,
  FolderX,
  Loader2,
  PlayCircle,
  UserRound,
} from "lucide-react";
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
  StatRow,
  type StatItem,
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
  LocaleCoverage,
  ProjectServer,
  RunError,
  UpPlanScope,
} from "../types/api";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useStoreSyncState } from "../hooks/useProjectStoreSync";
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
  /** Open Review, where the translations waiting for a person are. */
  onOpenReview?: () => void;
  /** Open Checks, where a failing check is explained. */
  onOpenChecks?: () => void;
}

/**
 * The convergence hero strip at the top of the project home (issue #1078 C4):
 * the project's standing as three labelled counts, split by who acts on them,
 * a "Bring up to date" that runs the shared `kapi up` engine, and a "Plan…"
 * pre-flight dialog. Translations to draft are the run's work; translations
 * waiting for review and languages held back from shipping are a person's, and
 * each count leads to where that person deals with it. The block cache is the
 * app's own business: while it catches up with the files the hero says the
 * files are being read, and never asks anyone to extract. When everything is
 * converged and the gates are green it renders the calm state with the button
 * disabled. Both derivations are cheap, read-only calls made on load and after
 * a run or a sync.
 */
export function ConvergenceHero({
  tabID,
  onBringUpToDate,
  convergence: propConvergence,
  plan: propPlan,
  server: propServer,
  lastRunError: propLastRunError,
  onOpenSettings,
  onOpenReview,
  onOpenChecks,
}: ConvergenceHeroProps) {
  const { hasActive, startJob, failActiveJob } = useJobFeed();
  const sync = useStoreSyncState(tabID);
  const [planOpen, setPlanOpen] = useState(false);
  const [heldOpen, setHeldOpen] = useState(false);
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
  // Source files that changed since the cache was built. The cache follows
  // them on its own (useProjectStoreSync), so this is a state the hero waits
  // out, never work it asks a person to start.
  const changed = (plan?.changedFiles ?? 0) + (plan?.removedFiles ?? 0);
  const cachePending = !!plan?.storeMissing || !!plan?.versionStale || changed > 0;
  const totalsPlanned = plan?.plan?.totals;
  // Every unit the plan prices, not only the ones with no target file. A pass is
  // driven by the content memory, so a produced unit the corpus cannot fill is
  // work the run will do — and a hero that read only the missing ones claimed a
  // project was up to date while the plan beside it quoted provider calls.
  const planned =
    (totalsPlanned?.missingTarget ?? 0) +
    (totalsPlanned?.stale ?? 0) +
    (totalsPlanned?.unanswered ?? 0);
  const parked = convergence?.review?.length ?? 0;
  const locales = convergence?.locales ?? [];
  const gated = locales.filter((l) => l.gated);
  // Three states, not two: gates green, gates unmet, and *no gates at all*.
  // `[].every()` is vacuously true, so folding the third into the first let an
  // ungated recipe — or a report we never received — assert "all gates green"
  // without a single gate having been evaluated. `hasGates` keeps the claim
  // honest: no gates means no claim, not a green one.
  const hasGates = gated.length > 0;
  // A scope that does not ship is held back, gate or no gate: below its ship
  // gate, or withheld by stale wording, a rejected translation, a failing check
  // or an unchecked term (convergence.LocaleCoverage).
  const heldBack = locales.filter((l) => !l.shippable);
  const notGatedLanguages = [
    ...new Set(
      locales
        .filter((l) => (l.shipState ? l.shipState === "not_gated" : !l.gated && l.shippable))
        .map((l) => l.locale),
    ),
  ];
  // A failed catch-up run leaves locales unproduced no matter what the
  // file-derived tally says, so it is drift in its own right — the state cannot
  // be trusted as converged until a run completes.
  const runFailed = lastRunError != null && lastRunError.kind !== "canceled";
  const upToDate = loaded && !cachePending && planned === 0 && !runFailed && heldBack.length === 0;
  // Reading the files: the cache is behind them and a sync is on its way or
  // under way. A sync that failed is reported below instead.
  const reading = loaded && cachePending && sync.error == null;
  // Whether a run has work: blocks to draft, a failed run to repeat, or a
  // held-back scope the run itself can clear (a redraft, a term check, a gate
  // short of drafted coverage). Otherwise the remaining work is a person's and
  // the run is not the primary action.
  const runHasWork =
    planned > 0 ||
    runFailed ||
    cachePending ||
    heldBack.some(
      (l) =>
        (l.staleAwaitingDraft ?? 0) > 0 ||
        (l.rejectedAwaitingDraft ?? 0) > 0 ||
        (l.termsNotChecked ?? 0) > 0 ||
        (l.pending ?? []).length > 0,
    );

  const counts: StatItem[] = [
    {
      id: "to-translate",
      value: planned,
      label: t(
        "{count, plural, one {block to translate} other {blocks to translate}}",
        "a count on the project home: blocks the run drafts",
        { count: planned },
      ),
      tone: planned > 0 ? "default" : "muted",
      hint: t("Bring up to date drafts these from the content memory and the AI model."),
    },
    {
      id: "waiting-for-review",
      value: parked,
      label: t(
        "waiting for review",
        "a count on the project home: translations waiting for a person",
      ),
      tone: parked > 0 ? "warning" : "muted",
      onClick: parked > 0 ? onOpenReview : undefined,
      hint: t("Translations a person approves before they ship."),
    },
    {
      id: "held-back",
      value: heldBack.length,
      label: t(
        "{count, plural, one {language held back} other {languages held back}}",
        "a count on the project home: collection languages that do not ship yet",
        { count: heldBack.length },
      ),
      tone: heldBack.length > 0 ? "warning" : "muted",
      onClick: heldBack.length > 0 ? () => setHeldOpen(true) : undefined,
      hint: t("A language of a collection that does not ship yet. Select it to see why."),
    },
  ];

  let headline: string;
  if (derivationError != null) headline = t("Could not determine project state");
  else if (!loaded) headline = t("Reading the project…");
  else if (reading) headline = t("Reading the project's files…");
  else if (runFailed) headline = t("The last run failed");
  else if (planned > 0) headline = t("Translations are missing");
  else if (parked > 0 || heldBack.length > 0) headline = t("Waiting for a person");
  else headline = t("Not up to date");

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
          ) : reading || !loaded ? (
            <Loader2 size={18} className="shrink-0 animate-spin text-muted-foreground" />
          ) : runHasWork ? (
            <PlayCircle size={18} className="shrink-0 text-primary" />
          ) : (
            <UserRound size={18} className="shrink-0 text-warning-ink" />
          )}
          <div className="min-w-0">
            {upToDate ? (
              <p className="text-sm font-medium" data-slot="hero-up-to-date">
                {/* Only claim the gates when there are gates to claim. */}
                {hasGates ? t("Up to date · all gates green") : t("Up to date")}
              </p>
            ) : (
              <p className="text-sm font-medium" data-slot="hero-drift-summary">
                {headline}
              </p>
            )}
            {(upToDate || plan?.plan?.subscription) && (
              <p className="text-xs text-muted-foreground">
                {upToDate &&
                  (hasGates
                    ? t("Every block is translated and every gated language ships.")
                    : t("Every block is translated. This recipe declares no ship gates."))}
                {upToDate && hasGates && notGatedLanguages.length > 0 && (
                  <span data-slot="hero-not-gated">
                    {" "}
                    {t("Not gated: {languages}.", { languages: notGatedLanguages.join(", ") })}
                  </span>
                )}
                {!upToDate && plan?.plan?.subscription && (
                  <span data-slot="hero-subscription">
                    {t("AI runs on your Claude subscription.")}
                  </span>
                )}
              </p>
            )}
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
            variant={runHasWork ? "default" : "outline"}
            onClick={launch}
            disabled={upToDate || hasActive || sync.syncing || !onBringUpToDate}
            data-slot="hero-bring-up-to-date"
            aria-label={t("Bring the project up to date")}
          >
            {hasActive ? <Loader2 size={13} className="animate-spin" /> : <PlayCircle size={13} />}
            {t("Bring up to date")}
          </Button>
        </div>
      </div>

      {loaded && !upToDate && !reading && derivationError == null && (
        <StatRow className="mt-4" data-slot="hero-counts" items={counts} />
      )}

      {/* The cache could not be rebuilt from the files. The counts above come
          from the files themselves, so they stand; what failed is named. */}
      {sync.error != null && cachePending && (
        <div data-slot="hero-sync-error" className="mt-3">
          <ErrorNotice
            error={sync.error}
            title={t("Could not read the project's files")}
            variant="panel"
            detailsLabel={t("Details")}
          />
        </div>
      )}

      {/* Declared files the report could not measure, such as a collection in a
          format no installed plugin supplies. A run converges the rest, so the
          files it left out are named here rather than absent from the numbers. */}
      {(convergence?.warnings ?? []).length > 0 && (
        <div
          data-slot="hero-unread"
          className="mt-3 rounded-md border border-amber-500/40 px-3 py-2 text-xs"
        >
          <p className="mb-1 font-medium">{t("Some declared content was not read")}</p>
          <ul className="space-y-0.5 text-muted-foreground">
            {(convergence?.warnings ?? []).map((warning) => (
              <li key={`${warning.source}#${warning.code}`}>{warning.message}</li>
            ))}
          </ul>
        </div>
      )}

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

      <HeldBackDialog
        open={heldOpen}
        onOpenChange={setHeldOpen}
        scopes={heldBack}
        onOpenReview={onOpenReview}
        onOpenChecks={onOpenChecks}
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

        {scopes.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground" data-slot="plan-empty">
            {t("Every block is translated.")}
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
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {s.memoryExact}
                  </TableCell>
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
                    {totals.memoryExact}
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

/** Why one held-back scope does not ship, as short reasons with counts. */
function heldBackReasons(l: LocaleCoverage): string[] {
  const out: string[] = [];
  for (const g of l.pending ?? []) {
    out.push(
      t("{actual}% {state}; the ship gate needs {required}%", {
        state: g.state,
        actual: g.actual,
        required: g.required,
      }),
    );
  }
  const n = (v: number | undefined) => v ?? 0;
  if (n(l.staleAwaitingReview) > 0) {
    out.push(
      t("{count} redrafted after a source change, waiting for review", {
        count: n(l.staleAwaitingReview),
      }),
    );
  }
  if (n(l.staleAwaitingDraft) > 0) {
    out.push(
      t("{count} with a source that changed after approval", { count: n(l.staleAwaitingDraft) }),
    );
  }
  if (n(l.rejectedAwaitingDraft) > 0) {
    out.push(t("{count} turned down by a reviewer", { count: n(l.rejectedAwaitingDraft) }));
  }
  if (n(l.failingChecks) > 0) {
    out.push(t("{count} failing a check", { count: n(l.failingChecks) }));
  }
  if (n(l.termsNotChecked) > 0) {
    out.push(t("{count} not checked against the terms", { count: n(l.termsNotChecked) }));
  }
  return out;
}

export interface HeldBackDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The scopes that do not ship. */
  scopes: readonly LocaleCoverage[];
  onOpenReview?: () => void;
  onOpenChecks?: () => void;
}

/**
 * The languages held back from shipping, one row per collection and language,
 * with what holds each one back. Review and Checks are where a person clears
 * what the run cannot.
 */
export function HeldBackDialog({
  open,
  onOpenChange,
  scopes,
  onOpenReview,
  onOpenChecks,
}: HeldBackDialogProps) {
  const failing = scopes.some((l) => (l.failingChecks ?? 0) > 0);
  const toGo = (go?: () => void) =>
    go
      ? () => {
          onOpenChange(false);
          go();
        }
      : undefined;
  const review = toGo(onOpenReview);
  const checks = toGo(onOpenChecks);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg" data-slot="held-back-dialog">
        <DialogHeader>
          <DialogTitle>{t("Held back from shipping")}</DialogTitle>
          <DialogDescription>
            {t("Bring up to date redrafts and checks what it can. The rest waits for a person.")}
          </DialogDescription>
        </DialogHeader>
        <Table className="text-xs">
          <TableHeader>
            <TableRow className="text-muted-foreground hover:bg-transparent">
              <TableHead className="h-auto py-1.5 pr-2 text-muted-foreground">
                {t("Language")}
              </TableHead>
              <TableHead className="h-auto py-1.5 pl-2 text-muted-foreground">
                {t("Held back by")}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {scopes.map((l, i) => {
              const reasons = heldBackReasons(l);
              return (
                <TableRow
                  key={`${l.collection ?? ""}-${l.locale}-${i}`}
                  className="border-border/50 align-top hover:bg-transparent"
                >
                  <TableCell className="py-1.5 pr-2">
                    {scopeName({
                      locale: l.locale,
                      collection: l.collection,
                      missingTarget: 0,
                      memoryExact: 0,
                      aiRemaining: 0,
                      tokenEstimate: 0,
                    })}
                  </TableCell>
                  <TableCell className="py-1.5 pl-2 whitespace-normal">
                    {reasons.length > 0 ? (
                      <ul className="space-y-0.5">
                        {reasons.map((r) => (
                          <li key={r}>{r}</li>
                        ))}
                      </ul>
                    ) : (
                      <span className="text-muted-foreground">{t("Not yet translated")}</span>
                    )}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
        <DialogFooter>
          {failing && checks && (
            <Button variant="outline" size="sm" onClick={checks}>
              {t("Open Checks")}
            </Button>
          )}
          {review && (
            <Button size="sm" onClick={review}>
              {t("Open Review")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
