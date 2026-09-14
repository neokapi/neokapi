// The review model's cards take the review model itself: the generated
// contract (`@neokapi/contract-types`, from core/review) is their prop type,
// so a field added to the Go struct reaches every card or fails to compile in
// the one that ignores it. Kapi Desktop reads the shape from the host, the
// platform reads it as the body of its REST review context, and neither maps
// it before handing it to a card.
//
// Two rows are structural rather than generated, because each is fed from
// outside the shape: a finding painted on the shared severity scale, which the
// platform also builds from its own check issues, and a term the source
// matches, which only the platform looks up.

import type { FindingTone } from "../../lib/finding-severity";

/** One term the unit's source matches, with the renderings the store mandates. */
export interface ReviewTermHitView {
  term: string;
  renderings?: string[];
  domain?: string;
}

/** One finding on the unit, from any checker, painted by its tone. */
export interface ReviewFindingView {
  /** A stable id for the row, used as its test hook. */
  id?: string;
  category?: string;
  /** The severity word the checker used, shown as given. */
  severity?: string;
  /** How hard it bites, on the shared scale. */
  tone: FindingTone;
  message: string;
  suggestion?: string;
  originalText?: string;
  /** Which side of the unit the finding is about; a source-side finding is marked. */
  field?: "source" | "target";
}

/**
 * A finding as a checker records it (core/check.Finding), with the two rows a
 * host may add beside it: which side it judged, and a structured replacement.
 * The generated `CheckFinding` satisfies this; so does Kapi Desktop's own
 * per-unit finding, which carries a side.
 */
export interface CheckFindingLike {
  category?: string;
  severity?: string;
  message: string;
  suggestion?: string;
  original_text?: string;
  replacement?: string;
  field?: "source" | "target";
}
