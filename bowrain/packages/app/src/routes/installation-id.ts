/**
 * Coerces the `installation_id` GitHub appends to the App's setup URL into a
 * string. The router's search parser JSON-parses values, so a numeric id like
 * `?installation_id=147350515` arrives as a number — accepting only strings
 * silently dropped it and stranded the post-install redirect on the fallback
 * card.
 */
export function coerceInstallationId(value: unknown): string | undefined {
  if (typeof value === "string" && value !== "") return value;
  if (typeof value === "number" && Number.isFinite(value)) return String(value);
  return undefined;
}
