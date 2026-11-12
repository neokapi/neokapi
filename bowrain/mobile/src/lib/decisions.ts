// Review decisions as TanStack Query mutations. Each decision is its own change
// set, sent in the order it was made. Offline, a decision waits as a paused
// mutation; the paused mutations are persisted on the device with the query
// cache and resume when the app is back online or returns to the foreground.
// The mutation function is registered with setMutationDefaults, so a decision
// restored after a restart still knows how to send itself.
import {
  approveSet,
  fixSet,
  isNetworkError,
  readOutcome,
  rejectSet,
  withdrawSet,
  type BowrainClient,
  type DeckCard,
  type FixReason,
  type ProjectScope,
} from "@neokapi/bowrain-client";
import type { Run } from "@neokapi/kapi-format";
import type { Mutation, QueryClient } from "@tanstack/react-query";

export const DECISION_KEY = ["decision"] as const;
export const WITHDRAW_KEY = ["withdraw"] as const;

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
  message?: string;
}

export interface WithdrawVars {
  id: string;
  scope: ProjectScope;
  card: DeckCard;
  revision: string;
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
    case "applied":
      return { id: v.id, outcome: "applied", revision: outcome.revision };
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
      if (res.outcome !== "applied") {
        addNotice(qc, { id: v.id, kind: res.outcome, card: v.card, message: res.message });
      }
      void qc.invalidateQueries({ queryKey: ["deck", v.scope.workspace, v.scope.project] });
      void qc.invalidateQueries({ queryKey: ["counts", v.scope.workspace, v.scope.project] });
    },
    onError: (err: unknown, v: DecisionVars) => {
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
      const outcome = readOutcome(await client().applyChanges(v.scope, withdrawSet(v.card, v.revision)));
      if (outcome.status !== "applied") {
        throw new Error(outcome.status === "refused" ? outcome.error.message : "the decision could not be undone");
      }
      return outcome;
    },
    scope: { id: "decisions" },
    retry: shouldRetry,
    onSettled: (_r: unknown, _e: unknown, v: WithdrawVars | undefined) => {
      if (!v) return;
      void qc.invalidateQueries({ queryKey: ["deck", v.scope.workspace, v.scope.project] });
    },
  });
}

type DecisionMutation = Mutation<DecisionResult, unknown, DecisionVars>;

function decisionMutations(qc: QueryClient): DecisionMutation[] {
  return qc
    .getMutationCache()
    .findAll({ mutationKey: DECISION_KEY })
    .map((m) => m as unknown as DecisionMutation)
    .filter((m) => m.state.variables)
    .sort((a, b) => (a.state.variables?.madeAt ?? 0) - (b.state.variables?.madeAt ?? 0));
}

/** The decisions not yet sent, oldest first. */
export function queuedDecisions(qc: QueryClient): DecisionVars[] {
  return decisionMutations(qc)
    .filter((m) => m.state.status === "pending" || m.state.isPaused)
    .map((m) => m.state.variables as DecisionVars);
}

/** What undoing the most recent decision would do, or null when there is nothing to undo. */
export type UndoPlan =
  | { kind: "unqueue"; vars: DecisionVars }
  | { kind: "withdraw"; vars: DecisionVars; revision: string }
  | null;

export function planUndo(qc: QueryClient): UndoPlan {
  const last = decisionMutations(qc).at(-1);
  const vars = last?.state.variables;
  if (!last || !vars) return null;
  if (last.state.isPaused) return { kind: "unqueue", vars };
  const data = last.state.data;
  // A correction that landed is not withdrawn: its content stays, so the undo
  // of a sent fix is a new edit on a desk.
  if (last.state.status === "success" && data?.outcome === "applied" && data.revision && vars.kind !== "fix") {
    return { kind: "withdraw", vars, revision: data.revision };
  }
  return null;
}

/**
 * Undo the most recent decision: drop it from the queue while it is still
 * waiting, or withdraw it with a new decision once it landed. Returns the card
 * that comes back, or null.
 */
export function undoLast(
  qc: QueryClient,
  withdraw: (v: WithdrawVars) => void,
): DeckCard | null {
  const plan = planUndo(qc);
  if (!plan) return null;
  const m = decisionMutations(qc).find((x) => x.state.variables?.id === plan.vars.id);
  if (plan.kind === "withdraw") {
    withdraw({ id: plan.vars.id, scope: plan.vars.scope, card: plan.vars.card, revision: plan.revision });
  }
  if (m) qc.getMutationCache().remove(m as unknown as Mutation);
  return plan.vars.card;
}
