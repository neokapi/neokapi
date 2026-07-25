import type { View } from "@neokapi/ui";

/**
 * Sidebar views for the web app: the shared @neokapi/ui views plus web-only
 * surfaces registered via AppShell's `extraNavItems` (currently the Locale
 * demand prototype).
 */
export type WorkspaceView = View | "locale-demand";

/** Map a URL sub-path to a sidebar View id. */
export function viewFromPath(pathname: string, workspace: string): WorkspaceView {
  const after = pathname.slice(`/${workspace}`.length);
  if (after.startsWith("/brand")) return "brand";
  if (after.startsWith("/termbase")) return "terms";
  if (after.startsWith("/memory")) return "memory";
  if (after.startsWith("/locale-demand")) return "locale-demand";
  if (after.startsWith("/auditlog")) return "auditlog";
  if (after.startsWith("/bin")) return "bin";
  if (after.startsWith("/settings")) return "settings";
  return "translate";
}
