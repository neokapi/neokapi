/**
 * Role-to-email mapping for the Acme CloudOps test personas.
 *
 * Agents reference teammates by role name (e.g. "pm", "translator-fr").
 * This module resolves those names to concrete @bowrain.test addresses
 * used by Mailpit in the local compose network.
 */

export const ROLE_EMAILS = {
  pm: "lisa.chen@bowrain.test",
  "brand-manager": "maria.santos@bowrain.test",
  developer: "alex.chen@bowrain.test",
  "translator-fr": "jeanpierre.dubois@bowrain.test",
  "translator-de": "katrin.weber@bowrain.test",
  "translator-ja": "yuki.tanaka@bowrain.test",
  qa: "taylor.kim@bowrain.test",
} as const satisfies Record<string, string>;

export type RoleName = keyof typeof ROLE_EMAILS | "all";

/**
 * Resolve a recipient string to one or more email addresses.
 *
 * - If `to` matches a known role name, the mapped address is returned.
 * - "all" returns every address in the mapping.
 * - Otherwise `to` is assumed to be a raw email address and returned as-is.
 */
export function resolveRecipient(to: string): string[] {
  if (to === "all") {
    return Object.values(ROLE_EMAILS);
  }

  const role = to.toLowerCase() as keyof typeof ROLE_EMAILS;
  if (role in ROLE_EMAILS) {
    return [ROLE_EMAILS[role]];
  }

  // Treat as a literal email address
  return [to];
}
