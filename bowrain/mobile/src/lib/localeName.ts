// The name of a language as a person reads it, in the app's UI language:
// "French" for fr, "Norwegian Bokmål" for nb. Hermes has not always carried
// Intl.DisplayNames, so the name is looked up only where the runtime has it,
// and a runtime without it, or a code it does not know, gives null. A caller
// then shows the code alone.

const cache = new Map<string, Intl.DisplayNames | null>();

function displayNames(uiLanguage: string): Intl.DisplayNames | null {
  let names = cache.get(uiLanguage);
  if (names !== undefined) return names;
  names = null;
  if (typeof Intl !== "undefined" && typeof Intl.DisplayNames === "function") {
    try {
      names = new Intl.DisplayNames([uiLanguage, "en"], { type: "language", fallback: "none" });
    } catch {
      names = null;
    }
  }
  cache.set(uiLanguage, names);
  return names;
}

/** The display name of `code` in `uiLanguage`, or null when none is known. */
export function localeName(code: string, uiLanguage: string): string | null {
  const names = displayNames(uiLanguage);
  if (!names) return null;
  try {
    const name = names.of(code.replace(/_/g, "-"));
    return name && name.toLowerCase() !== code.toLowerCase() ? name : null;
  } catch {
    // A code that is not a well-formed language tag.
    return null;
  }
}
