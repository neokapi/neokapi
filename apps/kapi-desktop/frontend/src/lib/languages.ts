import { isBareEntry, type KapiProject } from "../types/api";

/** One content item as the recipe resolves it: its target template and its
 *  own languages, else its collection's. */
function effectiveTargets(project: KapiProject): { target?: string; languages: string[] }[] {
  const out: { target?: string; languages: string[] }[] = [];
  for (const coll of project.collections ?? []) {
    // A bare entry is its own single item; its languages are the collection's.
    const items = isBareEntry(coll) ? [{ target: coll.target }] : (coll.content ?? []);
    for (const item of items) {
      const own = "target_languages" in item ? (item.target_languages ?? []) : [];
      out.push({
        target: item.target,
        languages: own.length > 0 ? own : (coll.target_languages ?? []),
      });
    }
  }
  return out;
}

/**
 * Whether the recipe names a target language anywhere its content could be
 * written to one: in `defaults.target_languages`, or on a collection or item
 * that also declares a target template. It is the rule of
 * `project.KapiProject.DeclaresTargetLanguages`, which the CLI's Languages
 * help group and `kapi up` read, applied to the recipe the tab already holds.
 * No file is read.
 *
 * Every surface that shows language work (the rail's Languages and Review,
 * the hub's Content memory, the Project page's coverage, and Bring up to date
 * on the Project page and in the runner) reads this one answer.
 */
export function declaresTargetLanguages(project: KapiProject | null | undefined): boolean {
  if (!project) return false;
  if ((project.defaults?.target_languages?.length ?? 0) > 0) return true;
  return effectiveTargets(project).some((i) => !!i.target && i.languages.length > 0);
}

/**
 * The target languages the recipe resolves to, in recipe order and without
 * repeats: `defaults.target_languages`, then the languages of every item that
 * declares a target template, each item's own before its collection's. It is
 * empty exactly when `declaresTargetLanguages` is false.
 */
export function resolvedTargetLanguages(project: KapiProject | null | undefined): string[] {
  if (!project) return [];
  const seen = new Set<string>((project.defaults?.target_languages ?? []).map(String));
  for (const item of effectiveTargets(project)) {
    if (!item.target) continue;
    for (const lang of item.languages) seen.add(String(lang));
  }
  return [...seen];
}
