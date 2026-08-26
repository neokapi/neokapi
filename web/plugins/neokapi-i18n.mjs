/**
 * Automatic i18n for this site's own React components.
 *
 * Docusaurus extracts by static analysis of `<Translate>` and `translate()`, so
 * a bare JSX literal is invisible to it: 57 files here carry 624 strings no
 * locale could reach. This transform reads JSX by the W3C HTML5 translatability
 * rules instead — the same table `core/translatability` embeds for the markdown
 * and mdx readers — so there is no wrapper to forget.
 *
 * It names the transform as a webpack LOADER rather than importing the
 * `./webpack` unplugin adapter, and this file imports nothing but node:path.
 * Both follow from one fact: Docusaurus evaluates its config and every plugin
 * module through jiti, unplugin's dist uses `import.meta.dirname`, and that is a
 * syntax error under jiti — so no module this config can reach may import the
 * adapter, directly or transitively. A loader is named by a path string that
 * webpack resolves and loads itself, well clear of the host's transpiler.
 *
 * Inline mode is entirely per-file, so the loader route gives up nothing this
 * site uses. What a loader cannot carry is the adapter's bundle-level work:
 * runtime mode's per-chunk manifest and the review overlay's HTML injection.
 *
 * Docusaurus's own theme components keep using code.json. Two catalogs is the
 * accepted cost of the zero-wrapper property; see strategy/backlog/016.
 */

import { readFileSync } from "node:fs";
import path from "node:path";

export default function neokapiI18nPlugin(context) {
  const { currentLocale, defaultLocale } = context.i18n;

  // The same file the extractor is given. A componentMap the two disagree on
  // produces hashes the transform cannot look up — the extractor writes one key,
  // the loader asks for another, and every string falls back to source.
  const sharedConfig = JSON.parse(
    readFileSync(path.resolve(context.siteDir, "i18n-react.config.json"), "utf8"),
  );

  return {
    name: "neokapi-i18n-react",

    configureWebpack() {
      // The default locale IS the source text, so transforming it would spend a
      // dictionary lookup on every string to arrive back where it started.
      if (currentLocale === defaultLocale) {
        return {};
      }

      return {
        module: {
          rules: [
            {
              // .ts as well as .tsx: a user-visible string does not stop
              // being one for sitting in a table of labels rather than in
              // markup. Nothing is swept in by the extension alone — a file
              // with no JSX and no t() yields nothing to translate.
              test: /\.[jt]sx?$/,
              // This site's own components only. Docusaurus's theme and every
              // dependency keep their own i18n.
              include: [path.resolve(context.siteDir, "src")],
              use: [
                {
                  loader: "@neokapi/i18n-react/loader",
                  options: {
                    // Docusaurus runs one webpack compilation per locale
                    // (client and server each) and carries the locale here, so
                    // inline mode gets the right answer in every one without a
                    // LOCALE env var.
                    locale: currentLocale,
                    translationsDir: path.resolve(context.siteDir, "i18n-runtime"),
                    // Must match what the extractor was given, or the hashes
                    // the transform looks up are not the hashes it wrote.
                    componentMap: sharedConfig.componentMap,
                  },
                },
              ],
            },
          ],
        },
      };
    },
  };
}
