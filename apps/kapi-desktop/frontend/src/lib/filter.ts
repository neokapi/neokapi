/**
 * Active Filter application helpers — pure functions shared by every view so a
 * filter narrows content and languages identically across the app and in runs.
 *
 * A filter narrows along three dimensions; an empty dimension means "all":
 *   - collections: keep only files in these collections
 *   - glob:        keep only files whose path matches this glob
 *   - languages:   keep only these target languages
 */
import { globToRegExp } from "@neokapi/kapi-lab/glob.ts";
import type { ProjectFilter } from "../types/api";

/** Minimal file shape the filter needs — matches FileMatch from the backend. */
export interface FilterableFile {
  path: string;
  relative?: string;
  collection?: string;
}

/** True when the filter narrows nothing (equivalent to "All"). */
export function isEmptyFilter(filter: ProjectFilter | null | undefined): boolean {
  if (!filter) return true;
  return (
    (filter.collections?.length ?? 0) === 0 &&
    !filter.glob?.trim() &&
    (filter.languages?.length ?? 0) === 0
  );
}

function fileGlob(glob: string): RegExp | undefined {
  const pattern = glob.trim();
  if (!pattern) return undefined;
  return globToRegExp(pattern.includes("/") ? pattern : `**/${pattern}`);
}

export function matchGlob(glob: string, path: string): boolean {
  return fileGlob(glob)?.test(path) ?? true;
}

/** Keep only files within the filter's collections and matching its glob. */
export function filterFiles<T extends FilterableFile>(
  files: T[],
  filter: ProjectFilter | null | undefined,
): T[] {
  if (isEmptyFilter(filter)) return files;
  const cols = filter?.collections ?? [];
  const glob = fileGlob(filter?.glob ?? "");
  return files.filter((f) => {
    if (cols.length && (!f.collection || !cols.includes(f.collection))) return false;
    return glob?.test(f.relative ?? f.path) ?? true;
  });
}

/** Keep only the filter's target languages (all when the filter names none). */
export function filterLanguages(
  languages: string[],
  filter: ProjectFilter | null | undefined,
): string[] {
  const want = filter?.languages ?? [];
  if (!want.length) return languages;
  return languages.filter((l) => want.includes(l));
}

/** Keep only the filter's collections (by name). */
export function filterCollectionNames(
  names: string[],
  filter: ProjectFilter | null | undefined,
): string[] {
  const want = filter?.collections ?? [];
  if (!want.length) return names;
  return names.filter((n) => want.includes(n));
}
