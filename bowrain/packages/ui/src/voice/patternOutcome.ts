import type { Pattern } from "./types";

/** What a match of a pattern does to a check, as the editors offer it. */
export type PatternOutcome = "fails" | "reports";

/**
 * A pattern's outcome: it fails only when it is marked `fails` and not
 * `advisory`. Mirror of Go core/profile's Pattern.Failing.
 */
export function patternOutcome(pattern: Pick<Pattern, "fails" | "advisory">): PatternOutcome {
  return pattern.fails && !pattern.advisory ? "fails" : "reports";
}

/**
 * The pattern with the outcome a person chose: `fails: true` for "fails", and
 * neither key for "reports", the default. `advisory` is cleared either way, so
 * the choice is the whole answer.
 */
export function withPatternOutcome(pattern: Pattern, outcome: string): Pattern {
  const { fails: _fails, advisory: _advisory, ...rest } = pattern;
  return outcome === "fails" ? { ...rest, fails: true } : rest;
}
