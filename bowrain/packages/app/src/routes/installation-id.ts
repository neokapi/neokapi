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

/**
 * Search-param validator: keeps a valid id in its ORIGINAL type so the router
 * re-serializes the URL unchanged (a coerced string would round-trip as a
 * JSON-quoted `?installation_id="123"`). Consumers coerce with
 * {@link coerceInstallationId} at the point of use.
 */
export function searchInstallationId(value: unknown): string | number | undefined {
  if (typeof value === "string" && value !== "") return value;
  if (typeof value === "number" && Number.isFinite(value)) return value;
  return undefined;
}

/**
 * Search-param validator for GitHub's `setup_action`. GitHub appends
 * `setup_action=install` on first install and `setup_action=update` when the
 * user changes the installation's repository access (with "Redirect on
 * update" enabled). The page treats both identically — the repo list is
 * re-read either way — so the value is kept only as a signal that the visit
 * came from a GitHub redirect (used by the missing-id diagnostics). Anything
 * unrecognised is dropped rather than round-tripped.
 */
export function searchSetupAction(value: unknown): "install" | "update" | undefined {
  return value === "install" || value === "update" ? value : undefined;
}

/**
 * Search-param validator for the signed setup state. Bowrain mints it before
 * sending the user to GitHub and GitHub echoes it back on the setup redirect;
 * it names the workspace that started the install, which the installation id
 * alone never does. Kept as the raw string so the router re-serializes it
 * unchanged — a coerced value would round-trip as a JSON-quoted `?state="..."`
 * and no longer verify.
 */
export function searchSetupState(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}
