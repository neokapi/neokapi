// What the Content memory page reads off the workspace and its entries, kept
// free of React so each rule is tested directly.
//
// The workspace's content memory answers one target language at a time: the
// server reads each entry as a pair, its source and its wording in the target
// language asked for. The page therefore offers the target languages the
// workspace's projects translate into, and asks for one of them.
import type { MemoryEntryInfo, ProjectInfo } from "../../types/api";
import { workspaceSourceLocale } from "../concepts/workspace-source-locale";

/** The languages a workspace's content memory is read in. */
export interface WorkspaceLanguages {
  /** The source language most of its projects declare, or "" when none does. */
  source: string;
  /** Every target language its projects declare, most widely declared first. */
  targets: string[];
}

/**
 * The workspace's languages, from its projects: the source most of them share,
 * and every target any of them declares, the ones more projects share first
 * and ties in the order the projects list them.
 */
export function workspaceLanguages(projects: ProjectInfo[] | undefined): WorkspaceLanguages {
  const source = workspaceSourceLocale(projects);
  const counts = new Map<string, number>();
  for (const project of projects ?? []) {
    for (const raw of project.target_languages ?? []) {
      const locale = raw.trim();
      if (!locale || locale === source) continue;
      counts.set(locale, (counts.get(locale) ?? 0) + 1);
    }
  }
  // Map keeps first-seen order, and Array.prototype.sort is stable, so a tie
  // keeps the order the projects listed the language in.
  const targets = [...counts.entries()].sort((a, b) => b[1] - a[1]).map(([locale]) => locale);
  return { source, targets };
}

/** A row's id in the list: the entry, and the language its target is in. */
export function pairId(entry: MemoryEntryInfo): string {
  return `${entry.id}::${entry.target_language}`;
}

/** The test id a row carries, which the walkthroughs and e2e suites find a pair by. */
export function pairTestId(entry: MemoryEntryInfo): string {
  return `tm-entry-${entry.id}${entry.target_language ? `-${entry.target_language}` : ""}`;
}

/** Whether an entry is held for one project rather than for the whole workspace. */
export function isProjectScoped(entry: MemoryEntryInfo): boolean {
  return !!entry.project_id;
}

/** The stream an entry belongs to, when it is not the main line. */
export function entryStream(entry: MemoryEntryInfo): string | undefined {
  const stream = entry.stream?.trim();
  return stream && stream !== "main" ? stream : undefined;
}
