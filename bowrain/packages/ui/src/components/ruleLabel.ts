/**
 * Naming the automation rule behind one execution.
 *
 * An execution record carries the rule's id: a stored rule's row id, or
 * `builtin:<name>` for one of the platform's own rules, which have no stored
 * row and so cannot be looked up by id. Both the execution history and a run's
 * steps name the rule this way, so they agree on what to call it.
 */

/** Marks an id that belongs to a platform rule rather than a stored one. */
const BUILT_IN_PREFIX = "builtin:";

/**
 * The name to show for the rule an execution came from.
 *
 * A stored rule resolves through `ruleNames` (the project's rules, by id) so a
 * renamed rule is named as it is called now. A platform rule's id carries its
 * name after the prefix. Anything left over falls back to the name recorded at
 * dispatch, and then to the raw id.
 */
export function ruleLabel(
  ruleId: string | undefined,
  ruleNames?: Record<string, string>,
  recordedName?: string,
): string {
  if (ruleId) {
    const stored = ruleNames?.[ruleId];
    if (stored) return stored;
    if (ruleId.startsWith(BUILT_IN_PREFIX)) return ruleId.slice(BUILT_IN_PREFIX.length);
  }
  return recordedName || ruleId || "";
}
