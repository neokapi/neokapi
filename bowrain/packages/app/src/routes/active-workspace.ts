import type { Workspace } from "@neokapi/ui";

/**
 * Which workspace a `/$workspace` URL resolves to.
 *
 * Three answers, and the third is the one that used to be missing. A member
 * removed from their last workspace, a deleted workspace, or an account that
 * outlived a data reset all arrive here with an empty list — a real state, not
 * an impossible one. Reading `workspaces[0].slug` in that case took the whole
 * app down to a blank page (BOWRAIN-8), because the guard above it only
 * handled the case where there WAS something to fall back to.
 */
export type WorkspaceResolution =
  /** The URL names a workspace this user has. */
  | { kind: "active"; workspace: Workspace }
  /** The URL names one they do not; fall back to their first. */
  | { kind: "fallback"; workspace: Workspace }
  /** They have none at all; the index route says so. */
  | { kind: "none" };

export function resolveActiveWorkspace(
  workspaces: Workspace[] | undefined,
  slug: string | undefined,
): WorkspaceResolution {
  if (!workspaces || workspaces.length === 0) return { kind: "none" };

  const match = workspaces.find((w) => w.slug === slug);
  if (match) return { kind: "active", workspace: match };

  // Non-empty by the check above, so this is a workspace and not undefined.
  return { kind: "fallback", workspace: workspaces[0]! };
}
