// The desktop's views for the places the project screens lead to.

import type { ScreenTarget } from "@neokapi/project-screens";

/**
 * The view that opens the Reader on a file, at one block when one is named:
 * "reader:<path>" or "reader:<path>:<block>", each part URI-encoded so a path
 * holding a colon stays one part.
 */
export function readerView(path: string, blockID?: string): string {
  const view = `reader:${encodeURIComponent(path)}`;
  return blockID ? `${view}:${encodeURIComponent(blockID)}` : view;
}

/** The file and block a "reader:" view names, or null for any other view. */
export function readerTargetOf(view: string): { path: string; blockID?: string } | null {
  const [base, path, block] = view.split(":");
  if (base !== "reader" || !path) return null;
  try {
    return {
      path: decodeURIComponent(path),
      ...(block ? { blockID: decodeURIComponent(block) } : {}),
    };
  } catch {
    return null;
  }
}

/** The view a target opens in a project tab. */
export function desktopView(target: ScreenTarget): string {
  switch (target.kind) {
    case "map":
      return "map";
    case "reader":
      return readerView(target.path, target.block);
    case "rules":
      return target.row ? `context:rules:${target.row}` : "context:rules";
    case "kept":
      return "context:atlas";
    case "checks":
      return "checks";
    case "blocks":
      return "context:explorer";
    case "setup":
      return "project-home";
    case "connect-assistant":
      return "context:agent";
  }
}
