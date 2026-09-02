/**
 * How hard a finding bites, and the one colour that says so.
 *
 * The scale is `core/check.Severity`, and its weights say what each rung means:
 * `neutral` (0) is informational, `minor` (1) is a soft preference, `major` (5)
 * is a clear violation a reviewer would act on, and `critical` (25) is release
 * blocking. Two of those stop a unit and two do not, so `major` and `critical`
 * take the destructive tone, `minor` takes warning, and `neutral` (or a word off
 * the scale) stays muted.
 *
 * Kapi Desktop and the Bowrain platform both read this, because the same word
 * from the same checker must not be amber on one surface and red on the other.
 * `packages/ui/docs/judgement-colours.md` records the contract: a finding is the
 * one place on a review page where a severity gets a colour at all.
 *
 * The server's check API grades its own issues `error` | `warning`, a shorter
 * scale over the same question. `checkIssueTone` maps it onto these three tones
 * so a surface listing check issues beside voice findings paints one list.
 */

/** The three tones a finding is drawn in. */
export type FindingTone = "destructive" | "warning" | "muted";

/**
 * The tone for a `core/check` severity. Unknown words take the muted tone: a
 * checker naming a rung this build has never heard of has said nothing about
 * how hard it bites, and painting it red would invent a verdict.
 */
export function findingSeverityTone(severity: string | undefined | null): FindingTone {
  switch ((severity ?? "").toLowerCase()) {
    case "critical":
    case "major":
      return "destructive";
    case "minor":
      return "warning";
    default:
      return "muted";
  }
}

/** Whether a `core/check` severity fails a unit rather than reporting on it. */
export function findingFails(severity: string | undefined | null): boolean {
  return findingSeverityTone(severity) === "destructive";
}

/** The tone for a server check issue, whose scale is `error` | `warning`. */
export function checkIssueTone(severity: string | undefined | null): FindingTone {
  switch ((severity ?? "").toLowerCase()) {
    case "error":
      return "destructive";
    case "warning":
      return "warning";
    default:
      return "muted";
  }
}

/** The badge a finding of this tone wears: border, fill and ink in one class. */
export function findingToneBadgeClass(tone: FindingTone): string {
  switch (tone) {
    case "destructive":
      return "border-destructive/40 bg-destructive/10 text-destructive";
    case "warning":
      return "border-warning/40 bg-warning/10 text-warning";
    default:
      return "text-muted-foreground";
  }
}

/** Ink alone, for a finding's message text and the icon beside it. */
export function findingToneTextClass(tone: FindingTone): string {
  switch (tone) {
    case "destructive":
      return "text-destructive";
    case "warning":
      return "text-warning";
    default:
      return "text-muted-foreground";
  }
}

/** The badge class for a `core/check` severity, in one call. */
export function findingSeverityBadgeClass(severity: string | undefined | null): string {
  return findingToneBadgeClass(findingSeverityTone(severity));
}
