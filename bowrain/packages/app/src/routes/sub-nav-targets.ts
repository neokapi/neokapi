/**
 * Where each secondary-nav item navigates.
 *
 * A table rather than a switch, because the invariant that matters is a
 * comparison: every item the sidebar renders must have a destination here.
 * Adding an item to `subNavConfig()` and adding its case to the handler were
 * two unrelated edits with nothing tying them together, and the failure mode was
 * silent — the item rendered, highlighted on hover, and went nowhere. Both
 * "Content memory" and "Locale demand" shipped that way.
 *
 * `subNavTargetsAreComplete` in sub-nav-targets.test.ts holds the two in step.
 */
export const subNavTargets = {
  // Context hub — the points you govern, and the graph you author at them.
  profiles: "/$workspace/context/profiles",
  explorer: "/$workspace/context/explorer",
  concepts: "/$workspace/context/concepts",
  voice: "/$workspace/context/voice",
  memory: "/$workspace/context/memory",
  changes: "/$workspace/context/changes",
  activity: "/$workspace/context/activity",

  // Insights — what the graph reports back. The overview lives under the
  // context section's route while locale demand has its own top-level path;
  // viewFromPath maps both to the insights view.
  dashboard: "/$workspace/context/dashboard",
  "locale-demand": "/$workspace/locale-demand",

  // Settings, plus the two workspace-level surfaces that appear in its menu.
  general: "/$workspace/settings",
  languages: "/$workspace/settings/languages",
  members: "/$workspace/settings/members",
  roles: "/$workspace/settings/roles",
  governance: "/$workspace/settings/governance",
  providers: "/$workspace/settings/providers",
  tokens: "/$workspace/settings/tokens",
  system: "/$workspace/settings/system",
  bravo: "/$workspace/settings/bravo",
  billing: "/$workspace/settings/billing",
  auditlog: "/$workspace/auditlog",
  bin: "/$workspace/bin",
} as const;

export type SubNavId = keyof typeof subNavTargets;

/** The destination for a sub-nav id, or undefined if it has none. */
export function subNavTarget(id: string): (typeof subNavTargets)[SubNavId] | undefined {
  return (subNavTargets as Record<string, (typeof subNavTargets)[SubNavId]>)[id];
}
