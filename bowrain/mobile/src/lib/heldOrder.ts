// The order a list showed first, kept as its items change (see heldLayout.ts).
/**
 * The order of `current` as first shown (`held`), with keys that arrived later after
 * them in their own order, and keys that left dropped. `held` is null before
 * the list was first shown.
 */
export function heldOrder(held: readonly string[] | null, current: readonly string[]): string[] {
  if (held === null) return [...current];
  const now = new Set(current);
  const kept = held.filter((k) => now.has(k));
  const seen = new Set(kept);
  return [...kept, ...current.filter((k) => !seen.has(k))];
}
