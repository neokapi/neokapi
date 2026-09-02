/**
 * How a finding's severity is painted on the Review surface.
 *
 * One map for both halves of the Checks card, the registered checkers and the
 * AI pre-review, so the same word carries the same colour whichever produced it.
 * Only a finding gets a severity colour: the layers above it draw the context
 * the model was given, and that context stays neutral however hard a rule bites.
 */
export function severityBadgeClass(severity: string | undefined): string {
  switch ((severity ?? "").toLowerCase()) {
    case "critical":
      return "border-destructive/40 bg-destructive/10 text-destructive";
    case "major":
      return "border-warning/40 bg-warning/10 text-warning";
    case "minor":
      return "border-warning/30 bg-warning/5 text-warning/90";
    default:
      return "text-muted-foreground";
  }
}
