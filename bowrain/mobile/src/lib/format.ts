// Numbers as the reader's language writes them.

/** A count with the language's grouping: 34,229 in English, 34 229 in Norwegian. */
export function formatCount(n: number, uiLanguage: string): string {
  try {
    return new Intl.NumberFormat(uiLanguage).format(n);
  } catch {
    return String(n);
  }
}
