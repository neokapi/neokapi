// How a decision's reason code reads to a person. The fix sheet offers the
// reasons with these phrases, and a decision row shows a past decision's
// reasons with the same ones. The phrases are the catalog's reasons.* keys.
import { DECISION_REASONS } from "@neokapi/contract-types";
import type { TFunction } from "i18next";

/** The catalog key of a reason code's phrase. */
export function reasonKey(code: string): string {
  return `reasons.${code}`;
}

/** A reason code as a phrase; a code with no phrase is shown as recorded. */
export function reasonLabel(t: TFunction, code: string): string {
  if (!(DECISION_REASONS as readonly string[]).includes(code)) return code;
  return t(reasonKey(code));
}
