// Review decisions: a log on the device, and TanStack Query mutations that
// send them. A new decision is held in the log for a few seconds (GRACE_MS),
// so a wrong swipe is taken back before anything reaches the server; then it
// becomes a mutation, its own change set, sent in the order it was made.
// Offline, it waits as a paused mutation. The log and the paused mutations
// are persisted with the query cache, and the mutation function is
// registered with setMutationDefaults, so a decision restored after a
// restart still knows how to send itself.
//
// The log keeps every decision made on this device for a day, with what
// became of it, so any of them can be undone from the Reviewed list: a held
// or queued decision is dropped, a sent one is withdrawn with a new decision.
import {
  approveSet,
  fixSet,
  isNetworkError,
  readOutcome,
  rejectSet,
  revisionsOf,
  withdrawSet,
  type BowrainClient,
  type DeckCard,
  type FixReason,
  type ProjectScope,
} from "@neokapi/bowrain-client";
import type { Run } from "@neokapi/kapi-format";
import {
  MutationObserver,
  useMutationState,
  useQuery,
  useQueryClient,
  type Mutation,
  type QueryClient,
} from "@tanstack/react-query";

export const DECISION_KEY = ["decision"] as const;
export const LOG_KEY = ["decision-log"] as const;

/** How long a new decision waits on the device, undoable, before it is sent. */
export const GRACE_MS = 5000;

/** How long the log keeps a decision: as long as the persisted cache lives. */
export const LOG_MAX_AGE = 24 * 60 * 60 * 1000;
export const WITHDRAW_KEY = ["withdraw"] as const;

/** The query key of the decisions a review entry of the feed counts. */
export function feedDecisionsKey(ws: string, project: string, entry: string) {
  return ["feed-decisions", ws, project, entry] as const;
}

export type DecisionKind = "approve" | "reject" | "fix";

/** A decision as it waits in the queue. Plain data, so it persists. */
export interface DecisionVars {
  id: string;
  scope: ProjectScope;
  card: DeckCard;
  kind: DecisionKind;
  /** The corrected runs, for a fix. */
  runs?: Run[];
  reasons?: FixReason[];
  note?: string;
  madeAt: number;
}

/** What the server made of a decision. */
export interface DecisionResult {
  id: string;
  /**
   * applied: it landed. held: the correction landed and the approval waits for
   * a second reviewer. stale: the text moved after the card was read. refused:
   * the server refused it, with the message.
   */
  outcome: "applied" | "held" | "stale" | "refused";
  /** The revision the decision left, which an undo names. */
  revision?: string;
  /** The revision it left on each copy the card stood for, by block. */
  copyRevisions?: Record<string, string>;
  message?: string;
}

export interface WithdrawVars {
  id: string;
  scope: ProjectScope;
  card: DeckCard;
  revision: string;
  copyRevisions?: Record<string, string>;
}

/** A notice the deck shows about a decision that did not land as made. */
export interface DecisionNotice {
  id: string;
  kind: "stale" | "held" | "refused";
  card: DeckCard;
  message?: string;
}

export const NOTICES_KEY = ["decision-notices"] as const;

let activeClient: BowrainClient | null = null;

/** The client queued decisions are sent with: the signed-in session's. */
export function setDecisionClient(client: BowrainClient | null): void {
  activeClient = client;
}

function client(): BowrainClient {
  if (!activeClient) throw new TypeError("not signed in");
  return activeClient;
}

/** Send one decision as its change set and read the result. */
export async function sendDecision(c: BowrainClient, v: DecisionVars): Promise<DecisionResult> {
  let set;
  if (v.kind === "approve") set = approveSet(v.card);
  else if (v.kind === "reject") set = rejectSet(v.card, v.reasons ?? [], v.note);
  else set = fixSet(v.card, v.runs ?? v.card.target, v.reasons ?? [], v.note);
  const outcome = readOutcome(await c.applyChanges(v.scope, set));
  switch (outcome.status) {
    case "applied": {
      const revisions = revisionsOf(set, outcome.result);
      const { [v.card.blockId]: own, ...copyRevisions } = revisions;
      return {
        id: v.id,
        outcome: "applied",
        revision: own ?? outcome.revision,
        ...(Object.keys(copyRevisions).length > 0 ? { copyRevisions } : {}),
      };
    }
    case "held":
      return { id: v.id, outcome: "held", message: outcome.error?.message };
    case "stale":
      return { id: v.id, outcome: "stale", message: outcome.op.error?.message };
    case "refused":
      return { id: v.id, outcome: "refused", message: outcome.error.message };
  }
}

/** Whether a failed send should be tried again: only when the server was not reached. */
export function shouldRetry(failureCount: number, error: unknown): boolean {
  return isNetworkError(error) && failureCount < 5;
}

function addNotice(qc: QueryClient, notice: DecisionNotice): void {
  qc.setQueryData<DecisionNotice[]>(NOTICES_KEY, (prev = []) => [
    ...prev.filter((n) => n.id !== notice.id),
    notice,
  ]);
}

export function dismissNotice(qc: QueryClient, id: string): void {
  qc.setQueryData<DecisionNotice[]>(NOTICES_KEY, (prev = []) => prev.filter((n) => n.id !== id));
}

/** Register how decisions and withdrawals send themselves. Call before the cache is restored. */
export function registerDecisionDefaults(qc: QueryClient): void {
  qc.setMutationDefaults(DECISION_KEY, {
    mutationFn: (v: DecisionVars) => sendDecision(client(), v),
    // One queue: decisions go out in the order they were made.
    scope: { id: "decisions" },
    retry: shouldRetry,
    onSuccess: (res: DecisionResult, v: DecisionVars) => {
      settle(qc, v.id, res);
      if (res.outcome !== "applied") {
        addNotice(qc, { id: v.id, kind: res.outcome, card: v.card, message: res.message });
      }
      void qc.invalidateQueries({ queryKey: ["deck", v.scope.workspace, v.scope.project] });
      void qc.invalidateQueries({ queryKey: ["review-inbox"] });
    },
    onError: (err: unknown, v: DecisionVars) => {
      settle(qc, v.id, {
        id: v.id,
        outcome: "refused",
        message: err instanceof Error ? err.message : undefined,
      });
      addNotice(qc, {
        id: v.id,
        kind: "refused",
        card: v.card,
        message: err instanceof Error ? err.message : undefined,
      });
    },
  });
  qc.setMutationDefaults(WITHDRAW_KEY, {
    mutationFn: async (v: WithdrawVars | undefined) => {
      if (!v) throw new Error("nothing to withdraw");
      const outcome = readOutcome(
        await client().applyChanges(v.scope, withdrawSet(v.card, v.revision, v.copyRevisions)),
      );
      if (outcome.status !== "applied") {
        throw new Error(
          outcome.status === "refused" ? outcome.error.message : "the decision could not be undone",
        );
      }
      return outcome;
    },
    scope: { id: "decisions" },
    retry: shouldRetry,
    onSuccess: (_r: unknown, v: WithdrawVars | undefined) => {
      if (v) setPhase(qc, v.id, "undone");
    },
    onError: (err: unknown, v: WithdrawVars | undefined) => {
      if (!v) return;
      // The decision stands: show it as sent again, and say why.
      setPhase(qc, v.id, "done");
      addNotice(qc, {
        id: `${v.id}:withdraw`,
        kind: "refused",
        card: v.card,
        message: err instanceof Error ? err.message : undefined,
      });
    },
    onSettled: (_r: unknown, _e: unknown, v: WithdrawVars | undefined) => {
      if (!v) return;
      void qc.invalidateQueries({
        queryKey: ["feed-decisions", v.scope.workspace, v.scope.project],
      });
      void qc.invalidateQueries({ queryKey: ["feed", v.scope.workspace, v.scope.project] });
      void qc.invalidateQueries({ queryKey: ["deck", v.scope.workspace, v.scope.project] });
      void qc.invalidateQueries({ queryKey: ["review-inbox"] });
    },
  });
}

type DecisionMutation = Mutation<DecisionResult, unknown, DecisionVars>;

function decisionMutations(qc: QueryClient): DecisionMutation[] {
  return qc
    .getMutationCache()
    .findAll({ mutationKey: DECISION_KEY })
    .map((m) => m as unknown as DecisionMutation)
    .filter((m) => m.state.variables);
}

function mutationOf(qc: QueryClient, id: string): DecisionMutation | undefined {
  return decisionMutations(qc).find((m) => m.state.variables?.id === id);
}

/**
 * Where a decision is. held: on the device, undoable, not sent yet. queued:
 * handed to the send queue (offline it waits there). done: the server
 * answered, and `result` says how. undoing: a withdrawal is on its way.
 * undone: taken back.
 */
export type DecisionPhase = "held" | "queued" | "done" | "undoing" | "undone";

export interface LoggedDecision {
  vars: DecisionVars;
  phase: DecisionPhase;
  result?: DecisionResult;
}

/** The decisions made on this device, oldest first. */
export function readLog(qc: QueryClient): LoggedDecision[] {
  return qc.getQueryData<LoggedDecision[]>(LOG_KEY) ?? [];
}

function writeLog(qc: QueryClient, f: (log: LoggedDecision[]) => LoggedDecision[]): void {
  const cutoff = Date.now() - LOG_MAX_AGE;
  qc.setQueryData<LoggedDecision[]>(LOG_KEY, (prev = []) =>
    f(prev).filter((e) => e.vars.madeAt >= cutoff || e.phase === "held" || e.phase === "queued"),
  );
}

function update(qc: QueryClient, id: string, f: (e: LoggedDecision) => LoggedDecision): void {
  writeLog(qc, (log) => log.map((e) => (e.vars.id === id ? f(e) : e)));
}

function setPhase(qc: QueryClient, id: string, phase: DecisionPhase): void {
  update(qc, id, (e) => ({ ...e, phase }));
}

function settle(qc: QueryClient, id: string, result: DecisionResult): void {
  update(qc, id, (e) => (e.phase === "undone" ? e : { ...e, phase: "done", result }));
}

/** Record a decision. It waits GRACE_MS on the device before it is sent. */
export function holdDecision(qc: QueryClient, vars: DecisionVars): void {
  writeLog(qc, (log) => [...log, { vars, phase: "held" }]);
}

function send(qc: QueryClient, vars: DecisionVars): void {
  void new MutationObserver<DecisionResult, unknown, DecisionVars>(qc, {
    mutationKey: DECISION_KEY,
  })
    .mutate(vars)
    .catch(() => undefined);
}

/**
 * Send the held decisions whose grace has passed, oldest first, or every held
 * decision when `all` is set (the app is leaving the foreground). Returns how
 * many were sent.
 */
export function releaseDue(qc: QueryClient, now: number, all = false): number {
  const due = readLog(qc).filter(
    (e) => e.phase === "held" && (all || e.vars.madeAt + GRACE_MS <= now),
  );
  if (due.length === 0) return 0;
  const ids = new Set(due.map((e) => e.vars.id));
  writeLog(qc, (log) =>
    log.map((e) => (ids.has(e.vars.id) ? { ...e, phase: "queued" as const } : e)),
  );
  for (const e of due) send(qc, e.vars);
  return due.length;
}

/**
 * After a restart: send again any decision the log handed to the queue but
 * whose mutation was not persisted (it was in flight when the app stopped).
 * Sending twice is safe: a decision names the revision it saw, so a repeat
 * is the same verdict or a stale one.
 */
export function resendLost(qc: QueryClient): void {
  for (const e of readLog(qc)) {
    if (e.phase === "queued" && !mutationOf(qc, e.vars.id)) send(qc, e.vars);
  }
}

/** The decisions not yet sent, held or queued, oldest first. */
export function queuedDecisions(qc: QueryClient): DecisionVars[] {
  const waiting = new Set(
    decisionMutations(qc)
      .filter((m) => m.state.status === "pending" || m.state.isPaused)
      .map((m) => m.state.variables?.id),
  );
  return readLog(qc)
    .filter((e) => e.phase === "held" || (e.phase === "queued" && waiting.has(e.vars.id)))
    .map((e) => e.vars);
}

/** What undoing one decision does, or null when it cannot be undone. */
export type UndoPlan =
  | { kind: "drop" }
  | { kind: "unqueue" }
  | { kind: "withdraw"; revision: string; copyRevisions?: Record<string, string> }
  | null;

export function planUndo(qc: QueryClient, entry: LoggedDecision): UndoPlan {
  switch (entry.phase) {
    case "held":
      return { kind: "drop" };
    case "queued": {
      // A decision on its way to the server can be undone once it lands.
      const m = mutationOf(qc, entry.vars.id);
      return m?.state.isPaused ? { kind: "unqueue" } : null;
    }
    case "done": {
      // A correction that landed is not withdrawn: its content stays, so the
      // undo of a sent fix is a new edit.
      const r = entry.result;
      if (r?.outcome === "applied" && r.revision && entry.vars.kind !== "fix") {
        return { kind: "withdraw", revision: r.revision, copyRevisions: r.copyRevisions };
      }
      return null;
    }
    default:
      return null;
  }
}

/**
 * Undo one decision: drop it while it is held or queued, or withdraw it with
 * a new decision once it landed. Returns the card that comes back, or null.
 */
export function undoDecision(
  qc: QueryClient,
  id: string,
  withdraw: (v: WithdrawVars) => void,
): DeckCard | null {
  const entry = readLog(qc).find((e) => e.vars.id === id);
  if (!entry) return null;
  const plan = planUndo(qc, entry);
  if (!plan) return null;
  const { vars } = entry;
  if (plan.kind === "withdraw") {
    setPhase(qc, id, "undoing");
    withdraw({
      id,
      scope: vars.scope,
      card: vars.card,
      revision: plan.revision,
      copyRevisions: plan.copyRevisions,
    });
  } else {
    const m = mutationOf(qc, id);
    if (m) qc.getMutationCache().remove(m as unknown as Mutation);
    setPhase(qc, id, "undone");
  }
  return vars.card;
}

/** The most recent decision a filter selects that is not taken back, or null. */
export function lastDecision(
  log: readonly LoggedDecision[],
  where: (e: LoggedDecision) => boolean,
): LoggedDecision | null {
  for (let i = log.length - 1; i >= 0; i--) {
    const e = log[i];
    if (e.phase === "undone" || e.phase === "undoing") continue;
    if (where(e)) return e;
  }
  return null;
}

/**
 * Whether a decision keeps its card out of the deck: while it is held or on
 * its way, and once it landed. A stale or refused one gives the card back.
 */
export function hidesCard(e: LoggedDecision): boolean {
  if (e.phase === "held" || e.phase === "queued") return true;
  if (e.phase === "done") return e.result?.outcome === "applied" || e.result?.outcome === "held";
  return false;
}

/**
 * The decision log, kept current. It also follows the send queue, so an
 * undo plan that depends on whether a decision is still waiting is read
 * again when that changes.
 */
export function useDecisionLog(): LoggedDecision[] {
  const qc = useQueryClient();
  useMutationState({
    filters: { mutationKey: DECISION_KEY },
    select: (m) => `${m.state.status}:${m.state.isPaused}`,
  });
  return useQuery<LoggedDecision[]>({
    queryKey: LOG_KEY,
    queryFn: () => readLog(qc),
    initialData: [],
    staleTime: Infinity,
    gcTime: LOG_MAX_AGE,
  }).data;
}
