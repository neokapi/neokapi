// Runtime smoke for the committed neokapi-i18n catalogs (make
// bowrain-app-translations / l10n-bowrain-app): the compiled dictionaries
// under public/translations/ must parse, load into the neokapi-i18n runtime,
// and resolve hash lookups — a stale or malformed catalog would otherwise
// only surface as untranslated (or garbled) text at runtime.
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it, vi, afterEach } from "vitest";
import { __t, loadTranslations, setTranslations } from "@neokapi/i18n-react/runtime";

const here = dirname(fileURLToPath(import.meta.url));
const translationsDir = join(here, "..", "..", "public", "translations");

function readCatalog(locale: string): Record<string, string> {
  return JSON.parse(readFileSync(join(translationsDir, `${locale}.json`), "utf8")) as Record<
    string,
    string
  >;
}

afterEach(() => {
  setTranslations("en", {}, { syncDocumentLocale: false });
  vi.restoreAllMocks();
});

describe("compiled translation catalogs", () => {
  it("qps catalog parses, is non-trivial, and resolves hash lookups", () => {
    const qps = readCatalog("qps");
    const entries = Object.entries(qps);
    // The pseudo catalog must cover the whole extraction — a sudden collapse
    // in entry count means extract/compile drifted from the source tree.
    expect(entries.length).toBeGreaterThan(1000);
    for (const [hash, value] of entries) {
      expect(hash).not.toBe("");
      expect(typeof value).toBe("string");
      expect(value).not.toBe("");
    }

    setTranslations("qps", qps, { syncDocumentLocale: false });
    const [hash, value] = entries[0];
    expect(__t(hash, "fallback-not-used")).toBe(value);
  });

  it("nb catalog carries the reviewed navigation strings", () => {
    const nb = readCatalog("nb");
    const values = Object.values(nb);
    // Seeded from context/memory/bowrain-app-nb.memory.json — the sidebar nav is the
    // high-traffic surface and must stay covered.
    expect(values).toContain("Prosjekter");
    expect(values).toContain("Innstillinger");
  });

  it("loadTranslations() fetches a catalog and activates the locale", async () => {
    const qps = readCatalog("qps");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify(qps))),
    );
    await loadTranslations("qps", "/translations/qps.json");
    expect(document.documentElement.lang).toBe("qps");
    const [hash, value] = Object.entries(qps)[0];
    expect(__t(hash, "fallback-not-used")).toBe(value);
    vi.unstubAllGlobals();
  });
});
