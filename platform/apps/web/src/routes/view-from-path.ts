import type { View } from "@neokapi/ui";

/** Map a URL sub-path to a sidebar View id. */
export function viewFromPath(pathname: string, workspace: string): View {
  const after = pathname.slice(`/${workspace}`.length);
  if (after.startsWith("/termbase")) return "termbase";
  if (after.startsWith("/memory")) return "memory";
  if (after.startsWith("/settings")) return "settings";
  return "translate";
}
