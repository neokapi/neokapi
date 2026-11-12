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
 * one action that runs the shared `kapi up` engine, and a "Plan…" pre-flight
 * dialog. The action is named for the work it does ("Translate 12 blocks",
 * "Run again", "Catch up") and is absent when the run has nothing to do. Translations to draft are the run's work; translations
 * waiting for review and languages held back from shipping are a person's, and
 * each count leads to where that person deals with it. The block cache is the
 * app's own business: while it catches up with the files the hero says the
 * files are being read, and never asks anyone to extract. When everything is
 * converged and the gates are green it renders the calm state with no action. Both derivations are cheap, read-only calls made on load and after
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
    (totalsPlanned?.missing_target ?? 0) +
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
        .filter((l) => (l.ship_state ? l.ship_state === "not_gated" : !l.gated && l.shippable))
        .map((l) => l.locale),
    ),
  ];
  // A failed catch-up run leaves locales unproduced no matter what the
  // file-derived tally says, so it is drift in its own right — the state cannot
  // be trusted as converged until a run completes.
  const runFailed = lastRunError != null && lastRunError.kind !== "canceled";
  // Target languages are set but no collection has a target path, so a run
  // has nowhere to write a translation. That is a setup step, not "up to date".
  const noTargets = !!plan?.plan?.no_targets;
  const upToDate =
    loaded && !cachePending && !noTargets && planned === 0 && !runFailed && heldBack.length === 0;
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
        (l.stale_awaiting_draft ?? 0) > 0 ||
        (l.rejected_awaiting_draft ?? 0) > 0 ||
        (l.terms_not_checked ?? 0) > 0 ||
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
      hint: t(
        "Exact content-memory matches are recycled as the files change. A run drafts the rest.",
      ),
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
  else if (noTargets) headline = t("No target folder set");
  else if (planned > 0) headline = t("Translations are missing");
  else if (parked > 0 || heldBack.length > 0) headline = t("Waiting for a person");
  else headline = t("Not up to date");

  // The run's action says what it will do. Blocks to draft come first; a
  // failed run with nothing new to draft is repeated; anything else the run
  // can clear (a redraft, a term check) is a catch-up.
  const actionLabel =
    planned > 0
      ? t(
          "{count, plural, one {Translate # block} other {Translate # blocks}}",
          "the project home's run button: drafts the blocks that have no translation",
          { count: planned },
        )
      : runFailed
        ? t("Run again", "the project home's run button after a failed run")
        : t("Catch up", "the project home's run button: the desktop form of kapi up");
  const fromMemory = totalsPlanned?.memory_exact ?? 0;
  const recycled = plan?.recycled?.blocks ?? 0;
  const fromDrafts = totalsPlanned?.drafts ?? 0;
  const byAI = totalsPlanned?.ai_remaining ?? 0;
  const tokens = totalsPlanned?.token_estimate ?? 0;

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
            {planned > 0 &&
              !reading &&
              loaded &&
              !noTargets &&
              (fromMemory > 0 || fromDrafts > 0 || byAI > 0) && (
                <p className="text-xs text-muted-foreground" data-slot="hero-run-size">
                  {[
                    fromMemory > 0 &&
                      t("{count} recycled", "how many blocks content-memory reuse fills", {
                        count: fromMemory,
                      }),
                    fromDrafts > 0 &&
                      t("{count} from stored drafts", "how many blocks a stored draft fills", {
                        count: fromDrafts,
                      }),
                    byAI > 0 &&
                      (plan?.plan?.subscription
                        ? t("{count} by the AI model", "how many blocks the AI model drafts", {
                            count: byAI,
                          })
                        : t(
                            "{count} by the AI model, about {tokens} tokens",
                            "how many blocks the AI model drafts, and the token estimate",
                            { count: byAI, tokens: tokens.toLocaleString() },
                          )),
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </p>
              )}
            {/* What the last sync filled on its own, so a target that appeared
                without a run has a visible source. */}
            {recycled > 0 && loaded && !reading && (
              <p className="text-xs text-muted-foreground" data-slot="hero-recycled">
                {t(
                  "{count, plural, one {# block recycled} other {# blocks recycled}}",
                  "targets the app filled from exact content-memory matches when the files changed",
                  { count: recycled },
                )}
              </p>
            )}
            {noTargets && !reading && loaded && (
              <p className="text-xs text-muted-foreground" data-slot="hero-no-targets">
                {t("Set where each collection's translations go.")}
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
            {/* The model a run uses is named in the Plan dialog, beside the
                work it will do; launching without one asks for one. */}
            {server?.connected && (
              <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                <VenueBadge server={server} />
              </div>
            )}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {noTargets && onOpenSettings && (
            <Button
              size="sm"
              onClick={() => onOpenSettings("project-settings:content")}
              data-slot="hero-set-targets"
            >
              {t("Set target folder")}
            </Button>
          )}
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
          {/* The run is offered only when it has work; a project waiting for a
              person leads to Review through its counts instead. */}
          {runHasWork && !upToDate && !noTargets && onBringUpToDate && (
            <Button
              size="sm"
              onClick={launch}
              disabled={hasActive || sync.syncing || !loaded}
              data-slot="hero-bring-up-to-date"
            >
              {hasActive ? (
                <Loader2 size={13} className="animate-spin" />
              ) : (
                <PlayCircle size={13} />
              )}
              {actionLabel}
            </Button>
          )}
        </div>
      </div>

      {loaded && !upToDate && !noTargets && !reading && derivationError == null && (
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
        onConfirm={onBringUpToDate && runHasWork && !noTargets ? launch : undefined}
        confirmLabel={actionLabel}
        confirmDisabled={hasActive || upToDate}
        tabID={tabID}
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
            "This project is connected to Bowrain, so kapi up runs on the server. Catching up here runs the same engine locally.",
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
  /** The confirm button's label: the same words as the action on the home. */
  confirmLabel?: string;
  confirmDisabled?: boolean;
  /** The project tab, so the dialog names the model the run will use. */
  tabID?: string;
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
  confirmLabel,
  confirmDisabled,
  tabID,
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

        {tabID !== undefined && (
          <div
            className="flex items-center gap-2 text-xs text-muted-foreground"
            data-slot="plan-model"
          >
            {t("Model")}
            <ActiveModelBadge tabID={tabID} />
          </div>
        )}

        {server?.connected && (
          <p
            className="flex items-center gap-1.5 text-xs text-muted-foreground"
            data-slot="plan-venue"
          >
            <Cloud size={12} className="shrink-0" />
            {server.host
              ? t(
                  "Connected to Bowrain ({host}): kapi up runs on the server; catching up here runs the same engine locally.",
                  { host: server.host },
                )
              : t(
                  "Connected to Bowrain: kapi up runs on the server; catching up here runs the same engine locally.",
                )}
          </p>
        )}

        {plan?.plan?.no_targets ? (
          <p className="py-2 text-sm text-muted-foreground" data-slot="plan-no-targets">
            {t("No collection has a target folder, so there is nothing to translate.")}
          </p>
        ) : scopes.length === 0 ? (
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
                  {t("Recycled")}
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
                    {s.missing_target}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {s.memory_exact}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {s.drafts ?? 0}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {s.ai_remaining}
                  </TableCell>
                  <TableCell className="py-1.5 pl-2 text-right tabular-nums">
                    {s.token_estimate}
                  </TableCell>
                </TableRow>
              ))}
              {totals && (
                <TableRow className="font-medium hover:bg-transparent" data-slot="plan-totals">
                  <TableCell className="py-1.5 pr-2">{t("Total")}</TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {totals.missing_target}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {totals.memory_exact}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {totals.drafts ?? 0}
                  </TableCell>
                  <TableCell className="px-2 py-1.5 text-right tabular-nums">
                    {totals.ai_remaining}
                  </TableCell>
                  <TableCell className="py-1.5 pl-2 text-right tabular-nums">
                    {totals.token_estimate}
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
              {confirmLabel ??
                t("Catch up", "the project home's run button: the desktop form of kapi up")}
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
  if (n(l.stale_awaiting_review) > 0) {
    out.push(
      t("{count} redrafted after a source change, waiting for review", {
        count: n(l.stale_awaiting_review),
      }),
    );
  }
  if (n(l.stale_awaiting_draft) > 0) {
    out.push(
      t("{count} with a source that changed after approval", { count: n(l.stale_awaiting_draft) }),
    );
  }
  if (n(l.rejected_awaiting_draft) > 0) {
    out.push(t("{count} turned down by a reviewer", { count: n(l.rejected_awaiting_draft) }));
  }
  if (n(l.failing_checks) > 0) {
    out.push(t("{count} failing a check", { count: n(l.failing_checks) }));
  }
  if (n(l.terms_not_checked) > 0) {
    out.push(t("{count} not checked against the terms", { count: n(l.terms_not_checked) }));
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
  const failing = scopes.some((l) => (l.failing_checks ?? 0) > 0);
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
            {t("Catch up redrafts and checks what it can. The rest waits for a person.")}
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
                      missing_target: 0,
                      memory_exact: 0,
                      ai_remaining: 0,
                      token_estimate: 0,
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
              {t("Open checks")}
            </Button>
          )}
          {review && (
            <Button size="sm" onClick={review}>
              {t("Open review")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
