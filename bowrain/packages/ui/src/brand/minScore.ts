import { DEFAULT_MIN_SCORE } from "./complianceBar";

/**
 * Reading the authored on-brand bar out of a profile-editor field.
 *
 * `min_score` is the one governance number a voice profile carries: the server
 * excludes a block scoring below it from the on-brand rate and refuses to
 * auto-approve it (core/profile's ComplianceBar, mirrored by `complianceBar`).
 * The editors expose it as free text so "unset" stays expressible — a blank
 * field is the default bar, not the number zero — and this module is the one
 * place that turns that text into what the API carries.
 */

/** Help text under the bar field; it names the default a blank field means. */
export const MIN_SCORE_HELP =
  `Blocks scoring below this bar are not on-brand and are never auto-approved. ` +
  `Leave blank for the default (${DEFAULT_MIN_SCORE}).`;

/** An authored bar, and whether the text it came from was a legal one. */
export interface ParsedMinScore {
  /**
   * The value to send. `undefined` means "no bar of its own" — a blank field or
   * a literal 0, both of which the server reads as the default. Sending
   * `undefined` is also how an existing bar is cleared back to the default: the
   * Go request field is `omitempty` and the handler writes whatever it binds.
   */
  value?: number;
  /** False for text that is not a whole number in 0–100; the editor blocks save. */
  valid: boolean;
}

/** Parse the bar field's text. Whitespace-only is blank, i.e. the default. */
export function parseMinScore(raw: string): ParsedMinScore {
  const text = raw.trim();
  if (text === "") return { valid: true };
  if (!/^\d+$/.test(text)) return { valid: false };
  const n = Number(text);
  if (n > 100) return { valid: false };
  return { value: n === 0 ? undefined : n, valid: true };
}

/** The editor field's initial text for a profile: blank when it sets no bar. */
export function minScoreFieldValue(minScore?: number): string {
  return minScore && minScore > 0 ? String(minScore) : "";
}
