/**
 * The reference dataset in the locale being built.
 *
 * The /tools, /formats, /commands and /models pages, and the per-entry
 * reference pages, print strings from @neokapi/reference-data: display names,
 * descriptions, command help, model notes. `make generate-reference-docs`
 * writes that dataset in English and, beside it, one variant per locale that
 * has a catalog (packages/reference-data/data/<locale>/), translated from the
 * catalogs the dogfood loop maintains and falling back to English string by
 * string where the catalog has nothing yet.
 *
 * Docusaurus runs one webpack compilation per locale and carries the locale
 * here, so the swap happens where the JSON is read: a loader on the dataset
 * files returns the variant's bytes when one exists for the current locale.
 * The components keep importing the English module and never learn which
 * locale they are rendering; the default locale is the source text and is
 * left alone, as is a locale with no variant on disk.
 *
 * A loader rather than an alias because the dataset is imported by relative
 * path from inside the package, and an alias matches the request as written
 * rather than the file it resolves to. Only files directly under data/ match:
 * a variant is never itself swapped.
 */

import { existsSync } from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";

const require = createRequire(import.meta.url);

export default function referenceLocalePlugin(context) {
  const { currentLocale, defaultLocale } = context.i18n;

  return {
    name: "neokapi-reference-locale",

    configureWebpack() {
      if (currentLocale === defaultLocale) {
        return {};
      }
      const dataDir = path.dirname(require.resolve("@neokapi/reference-data/data/tools.json"));
      if (!existsSync(path.join(dataDir, currentLocale))) {
        console.warn(
          `[reference-locale] no reference dataset variant for ${currentLocale} under ${dataDir}; the reference pages render English`,
        );
        return {};
      }
      return {
        module: {
          rules: [
            {
              test: /[\\/]reference-data[\\/]data[\\/][^\\/]+\.json$/,
              type: "json",
              use: [
                {
                  loader: path.resolve(context.siteDir, "plugins/reference-locale-loader.cjs"),
                  options: { locale: currentLocale },
                },
              ],
            },
          ],
        },
      };
    },
  };
}
