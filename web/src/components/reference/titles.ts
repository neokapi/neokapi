// Titles for the static per-entry reference pages.
//
// A reference page is titled after its entry and the category it belongs to —
// "JSON format", "Segment tool". Some display names carry the category word
// already ("TSV Format", "Open Document Format"), and appending it to those
// produced the doubled word `kapi check` reports as `hygiene.doubled-word`:
// "TSV Format format".
//
// The category is named once. A display name whose LAST WORD is the category
// has named it; a name that merely ends in those letters ("ICU MessageFormat")
// has not — the word there is the name of a specification, and the category
// still needs saying.

/**
 * Title an entry for its category, naming the category once.
 *
 * `referenceTitle("TSV Format", "format")` → `"TSV Format"`;
 * `referenceTitle("JSON", "format")` → `"JSON format"`.
 */
export function referenceTitle(displayName: string, category: string): string {
  const name = displayName.trim();
  const words = name.split(/\s+/);
  const last = words[words.length - 1] ?? "";
  if (last.toLowerCase() === category.toLowerCase()) return name;
  return `${name} ${category}`;
}
