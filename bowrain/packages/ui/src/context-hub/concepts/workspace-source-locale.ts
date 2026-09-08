// The locale a workspace authors in, read off its projects (AGPL-3.0). Concepts
// are workspace-scoped and carry no source language of their own, so the Brand
// hub names a concept in the language its projects are written in: the
// `default_source_language` most of them share.
import type { ProjectInfo } from "../../types/api";

/**
 * The source locale most of `projects` declare, or an empty string when they
 * declare none. A tie goes to the locale that sorts first, so the same set of
 * projects always yields the same answer.
 */
export function workspaceSourceLocale(projects: ProjectInfo[] | undefined): string {
  if (!projects?.length) return "";
  const counts = new Map<string, number>();
  for (const project of projects) {
    const locale = project.default_source_language?.trim();
    if (locale) counts.set(locale, (counts.get(locale) ?? 0) + 1);
  }
  let best = "";
  let bestCount = 0;
  for (const [locale, count] of counts) {
    if (count > bestCount || (count === bestCount && locale < best)) {
      best = locale;
      bestCount = count;
    }
  }
  return best;
}
