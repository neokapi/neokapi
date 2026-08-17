/**
 * The directory prefix a set of item names shares.
 *
 * An item's name is its identity — the path the recipe's glob claimed it under,
 * which the server addresses it by and every route carries. That path opens
 * with the collection's `base:` (`bowrain/packages/app/…`), so a list of a
 * collection's items repeats, on every row, the one part of the path that
 * cannot tell two of them apart.
 *
 * Naming the prefix once and showing each item relative to it is a *reading* of
 * the names, never a change to them: the full name stays in the row's tooltip
 * and is what every callback carries.
 *
 * Mirrors `commonItemBase` in bowrain/server/editor.go, which computes the same
 * prefix for the overview's paged item list — the two surfaces must trim alike,
 * and this one can be exact because the source page holds the whole list.
 */

/**
 * The prefix every name shares, with a trailing slash, or "" when they share
 * none. Whole segments only: "docs/api" and "docs/apps" share "docs/", not
 * "docs/ap". A single item contributes its own directory, which is what makes a
 * one-file collection read as the file rather than as the path to it.
 */
export function commonItemBase(names: string[]): string {
  if (names.length === 0) return "";
  const dirOf = (name: string): string[] => {
    const segments = name.replace(/^\/+/, "").split("/");
    return segments.slice(0, -1); // drop the file's own name
  };

  let shared = dirOf(names[0]);
  for (const name of names.slice(1)) {
    if (shared.length === 0) return "";
    const segments = dirOf(name);
    if (segments.length < shared.length) shared = shared.slice(0, segments.length);
    for (let i = 0; i < shared.length; i++) {
      if (segments[i] !== shared[i]) {
        shared = shared.slice(0, i);
        break;
      }
    }
  }
  return shared.length === 0 ? "" : `${shared.join("/")}/`;
}

/**
 * A name as it reads under `base`. A name the base does not actually prefix is
 * left whole rather than half-trimmed — the base is a claim about the scope,
 * and an item outside it is better shown in full than shown wrong.
 */
export function relativeItemName(name: string, base: string): string {
  return base && name.startsWith(base) ? name.slice(base.length) : name;
}
