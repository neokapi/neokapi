// Webpack loader: return the locale variant of a reference dataset file when
// one exists beside it. See reference-locale.mjs for why it is a loader.

const { existsSync, readFileSync } = require("node:fs");
const path = require("node:path");

/**
 * The path of `resourcePath`'s variant for `locale`: the same file name in
 * the sibling `<locale>/` directory. Null when the file already is a variant,
 * so a variant is never looked up under itself.
 */
function localeVariantPath(resourcePath, locale) {
  const dir = path.dirname(resourcePath);
  if (path.basename(dir) === locale) return null;
  return path.join(dir, locale, path.basename(resourcePath));
}

/**
 * Swap a dataset file for its locale variant. The variant is declared as a
 * dependency so a rebuilt variant invalidates the cached module: webpack keys
 * its cache on the source and the options, and would otherwise not know the
 * loader read a second file.
 */
function referenceLocaleLoader(source) {
  const { locale } = this.getOptions();
  const variant = localeVariantPath(this.resourcePath, locale);
  if (!variant || !existsSync(variant)) return source;
  this.addDependency(variant);
  return readFileSync(variant, "utf8");
}

module.exports = referenceLocaleLoader;
module.exports.localeVariantPath = localeVariantPath;
