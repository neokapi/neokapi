import type { TFunction } from "i18next";

// The query key of the languages a person follows in a project, shared by the
// languages screen and anything that changes them.
export function reviewLanguagesKey(ws: string, project: string) {
  return ["review-languages", ws, project] as const;
}

/**
 * The header chip's label: the one language followed everywhere, or the
 * first and how many more. The languages come in the order the inbox lists
 * its projects, each once.
 */
export function followedChipLabel(
  followed: string[][],
  label: (code: string) => string,
  t: TFunction,
): string {
  const all = [...new Set(followed.flat())];
  if (all.length === 0) return t("follow.chipNone");
  if (all.length === 1) return label(all[0]!);
  return t("follow.chipMore", { first: label(all[0]!), count: all.length - 1 });
}
