// Runtime smoke for the compiled neokapi-i18n catalogs: the dictionaries under
// public/translations/ must parse, load into the neokapi-i18n runtime, and
// resolve hash lookups. A stale or malformed catalog would otherwise only
// surface as untranslated (or garbled) text at runtime.
//
// Shape, never coverage. The pseudo locale is build output derived from the
// source extraction (`make bowrain-app-qps`, which the vite build runs), so a
// collapse in its entry count means extract/compile drifted and is a real
// failure. A checkout that has not built the app has no qps dictionary, and
// those cases skip. A target locale is the dogfood loop's output and is
// routinely empty or partial while translation catches up with a source
// change. That is normal drift the loop absorbs, never a build failure.
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it, vi, afterEach } from "vite-plus/test";
import { __t, loadTranslations, setTranslations } from "@neokapi/i18n-react/runtime";

const here = dirname(fileURLToPath(import.meta.url));
const translationsDir = join(here, "..", "..", "public", "translations");

function readCatalog(locale: string): Record<string, string> {
  return JSON.parse(readFileSync(join(translationsDir, `${locale}.json`), "utf8")) as Record<
    string,
    string
  >;
}

const QPS_MISSING = "no qps dictionary; `make bowrain-app-qps` or a vite build writes it";

function hasCatalog(locale: string): boolean {
  return existsSync(join(translationsDir, `${locale}.json`));
}

// readTargetCatalog reads a target-locale catalog that may not have been
// generated at all: a missing file and an empty one mean the same thing.
function readTargetCatalog(locale: string): Record<string, string> {
  return hasCatalog(locale) ? readCatalog(locale) : {};
}

// expectLoadable asserts a catalog's shape: every entry is a non-empty hash
// mapped to a non-empty string, and the runtime resolves a lookup against it.
function expectLoadable(locale: string, catalog: Record<string, string>): void {
  const entries = Object.entries(catalog);
  for (const [hash, value] of entries) {
    expect(hash).not.toBe("");
    expect(typeof value).toBe("string");
    expect(value).not.toBe("");
  }
  setTranslations(locale, catalog, { syncDocumentLocale: false });
  const [hash, value] = entries[0];
  expect(__t(hash, "fallback-not-used")).toBe(value);
}

afterEach(() => {
  setTranslations("en", {}, { syncDocumentLocale: false });
  vi.restoreAllMocks();
});

describe("compiled translation catalogs", () => {
  it("qps catalog parses, is non-trivial, and resolves hash lookups", (ctx) => {
    if (!hasCatalog("qps")) {
      ctx.skip(QPS_MISSING);
      return;
    }
    const qps = readCatalog("qps");
    // The pseudo catalog must cover the whole extraction. A sudden collapse
    // in entry count means extract/compile drifted from the source tree.
    expect(Object.keys(qps).length).toBeGreaterThan(1000);
    expectLoadable("qps", qps);
  });

  it("nb catalog is loadable when it carries entries", (ctx) => {
    const nb = readTargetCatalog("nb");
    if (Object.keys(nb).length === 0) {
      ctx.skip("nb catalog is empty; `make l10n` fills it from the dogfood loop");
      return;
    }
    // Whatever nb does carry must be usable. How much it carries is the loop's
    // business, so a partial catalog passes exactly as a complete one does.
    expectLoadable("nb", nb);
  });

  it("loadTranslations() fetches a catalog and activates the locale", async (ctx) => {
    if (!hasCatalog("qps")) {
      ctx.skip(QPS_MISSING);
      return;
    }
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
