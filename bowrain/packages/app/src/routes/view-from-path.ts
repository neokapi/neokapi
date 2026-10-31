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
  if (after.startsWith("/context/dashboard")) return "insights";
  if (after.startsWith("/locale-demand")) return "insights";
  if (after.startsWith("/context")) return "context";
  if (after.startsWith("/terms")) return "terms";
  if (after.startsWith("/memory")) return "memory";
  if (after.startsWith("/auditlog")) return "auditlog";
  if (after.startsWith("/bin")) return "bin";
  if (after.startsWith("/settings")) return "settings";
  return "translate";
}

/** A project's sections, by the id the secondary panel gives each. */
export type ProjectSection =
  | "map"
  | "dashboard"
  | "rules"
  | "source"
  | "automations"
  | "runs"
  | "connectors";

/**
 * The project section a URL is on, or undefined outside a project. The Reader
 * is reached from the Map, so it lights the Map; a per-file editor lights the
 * Overview.
 */
export function projectViewFromPath(
  pathname: string,
  workspace: string,
): ProjectSection | undefined {
  const prefix = `/${workspace}/p/`;
  if (!pathname.startsWith(prefix)) return undefined;
  const parts = pathname.slice(prefix.length).split("/");
  if (parts.length < 3 || parts[1] !== "s") return undefined;
  switch (parts[3]) {
    case "map":
    case "read":
      return "map";
    case "rules":
    case "source":
    case "automations":
    case "runs":
    case "connectors":
      return parts[3];
    default:
      return "dashboard";
  }
}
