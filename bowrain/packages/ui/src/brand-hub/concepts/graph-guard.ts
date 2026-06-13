// The readability guard for the concept graph view (AD-021). The graph is the
// one navigator surface that does not paginate, so once a workspace's vocabulary
// outgrows what a single force-directed canvas can show legibly, the view refuses
// to draw a hairball and instead asks the steward to focus on a concept or narrow
// by a filter. The server already caps the payload and flags it `truncated`; this
// guard shares that intent and is kept pure so the decision is unit-testable and
// the component stays declarative.

/**
 * The largest node count the graph canvas renders legibly without a focus or
 * filter. Mirrors the server's default node cap, so the wide-open view guards
 * exactly when the server had to truncate a typical request.
 */
export const GRAPH_READABLE_NODE_LIMIT = 60;

export interface GraphGuardInput {
  /** The server capped the selection — more concepts exist than were returned. */
  truncated: boolean;
  /** Number of nodes in the returned payload. */
  nodeCount: number;
  /** A concept is focused (search / neighbourhood) — the view is already scoped. */
  hasFocus: boolean;
  /** A scope filter (market, as-of, …) is active — the view is already narrowed. */
  hasFilter: boolean;
}

/**
 * shouldGuardGraph decides whether to show the focus-or-filter guard instead of
 * the canvas. The guard applies only to the wide-open view: the moment the
 * steward focuses a concept or sets a filter, the graph renders normally, however
 * large the underlying vocabulary. Within the wide-open view it guards when the
 * server truncated the payload, or the node count exceeds the readable limit.
 */
export function shouldGuardGraph({
  truncated,
  nodeCount,
  hasFocus,
  hasFilter,
}: GraphGuardInput): boolean {
  if (hasFocus || hasFilter) return false;
  return truncated || nodeCount > GRAPH_READABLE_NODE_LIMIT;
}
