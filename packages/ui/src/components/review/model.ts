// The one projection the review model needs before a card can draw it: a
// checker's findings painted on the shared severity scale. Every other layer
// is a card prop as the host serialises it.

import { findingSeverityTone } from "../../lib/finding-severity";
import type { CheckFindingLike, ReviewFindingView } from "./types";

/**
 * A checker's findings, painted on the shared severity scale, with what each
 * was raised against and what to say instead. `idPrefix` names the rows so a
 * surface's tests and its anchored marks can refer to one; a surface listing
 * findings from two checkers gives each list its own prefix.
 */
export function checkFindingViews(
  findings: readonly CheckFindingLike[],
  idPrefix = "finding",
): ReviewFindingView[] {
  return findings.map((f, i) => ({
    id: `${idPrefix}-${i}`,
    category: f.category,
    severity: f.severity,
    tone: findingSeverityTone(f.severity),
    message: f.message,
    suggestion: f.suggestion ?? f.replacement,
    originalText: f.original_text,
    field: f.field,
  }));
}
