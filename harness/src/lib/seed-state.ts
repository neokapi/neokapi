/**
 * What the seeded recording project has to look like before a walk can start a
 * convergence run on camera, expressed over the server's own pre-flight
 * estimate (`GET /:ws/:project/convergence/estimate`).
 *
 * `ConvergenceRunNowDialog` offers "Translate all now" only when work is
 * pending over the READY source, and the server answers the transport scope 204
 * without creating a run. So a project whose source is still held by its gate,
 * or whose every locale is already covered, leaves the automations walk waiting
 * for a run row that no run will ever produce (#2597).
 *
 * The predicates live here rather than in the seed script so they can be tested
 * without a stack: the seed reads the estimate, acts on these, and reads it
 * again to prove the state it claims to have left.
 */

/** The source-first readiness split the estimate leads with. */
export interface SourceReadiness {
  /** The project's source gate: `checked` by default, `none` when opted out. */
  gate: string;
  total: number;
  ready: number;
  held: number;
}

/** The estimate's shape, to the fields the seed reads. */
export interface ConvergenceEstimate {
  source: SourceReadiness;
  totals: { pending: number };
}

/** One block as the editor's block list returns it. */
export interface EditorBlock {
  id: string;
  translatable?: boolean;
  targets?: Record<string, { text?: string } | undefined>;
}

/** Whether any source block still sits below the project's gate. */
export function needsSettle(est: ConvergenceEstimate): boolean {
  return est.source.held > 0;
}

/** Whether every locale is already covered over the ready source. */
export function needsPendingWork(est: ConvergenceEstimate): boolean {
  return est.totals.pending === 0;
}

/** Whether the project can start a run that reaches the runs table. */
export function isRunnable(est: ConvergenceEstimate): boolean {
  return !needsSettle(est) && !needsPendingWork(est);
}

/** One line for the seed log, naming whichever half is missing. */
export function describeReadiness(est: ConvergenceEstimate): string {
  const { gate, total, ready, held } = est.source;
  if (held > 0) return `${held} of ${total} source block(s) held by the "${gate}" gate`;
  if (est.totals.pending === 0) return `all ${total} source block(s) ready, and every locale already covered`;
  return `all ${total} source block(s) ready, ${est.totals.pending} unit(s) pending`;
}

/**
 * The blocks whose target in `locale` carries text, which is what the seed
 * clears to hand the walk its pending work back after a take translated it.
 * A block with no target, or one holding only whitespace, is pending already.
 */
export function targetsToClear(blocks: readonly EditorBlock[], locale: string): string[] {
  const ids: string[] = [];
  for (const b of blocks) {
    if (!b || b.translatable === false) continue;
    if (!(b.targets?.[locale]?.text ?? "").trim()) continue;
    ids.push(b.id);
  }
  return ids;
}
