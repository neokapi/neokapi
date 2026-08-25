/**
 * A plain webpack loader, for bundlers whose host loads its config through a
 * transpiler that cannot evaluate ESM.
 *
 * The `./webpack` entry point is the one to reach for normally: it is the
 * unplugin adapter and carries the bundle-level hooks (the chunk manifest for
 * runtime mode, the review overlay's HTML injection). This entry point carries
 * only the per-file transform, and exists because some hosts cannot load the
 * other one at all.
 *
 * Docusaurus is the case in hand. It evaluates `docusaurus.config.ts` and every
 * plugin module through jiti, and unplugin's dist uses `import.meta.dirname`,
 * which is a syntax error under it — so a Docusaurus site cannot import the
 * unplugin adapter from anywhere its config can reach. A loader is named by a
 * path STRING in `module.rules`, and webpack resolves and loads it itself, so
 * nothing about the transform passes through the host's transpiler.
 *
 * What is given up by using this instead of `./webpack`: runtime mode's
 * per-chunk manifest and the review overlay's script injection, both of which
 * are bundle-level rather than per-file. Inline mode — bake the target locale
 * in at build time — is entirely per-file and works exactly the same.
 */

import { join } from "node:path";

import type { PluginOptions } from "../types.ts";
import { transform } from "./transform.ts";

/** The `this` a webpack loader is called with, narrowed to what is used. */
interface LoaderContext {
  resourcePath: string;
  getOptions?: () => PluginOptions;
  query?: PluginOptions | string;
  cacheable?: (flag: boolean) => void;
  /** Declares a file whose contents this module's output depends on. */
  addDependency?: (file: string) => void;
}

/**
 * Transform one module's source. Returns the input untouched when the file
 * holds nothing translatable, so a rule can be broad without cost.
 */
export default function neokapiI18nLoader(this: LoaderContext, code: string): string {
  this.cacheable?.(true);

  const options: PluginOptions =
    (this.getOptions?.() as PluginOptions) ??
    (typeof this.query === "object" && this.query !== null ? this.query : {});

  // This module's output depends on the translation dictionary as much as on
  // its own source, and webpack has no way to know that: it hashes the file it
  // handed us and its loader options, so a REBUILT dictionary with the same
  // source leaves the cached output in place. That is not a stale-cache
  // nuisance, it is wrong output — an updated translation silently never
  // reaches the page, and the build reports success.
  const dir = options.translationsDir || "./translations";
  for (const locale of [options.locale, ...(options.fallbackLocales ?? [])]) {
    if (locale) {
      this.addDependency?.(join(dir, `${locale}.json`));
    }
  }

  const result = transform(code, this.resourcePath, options);
  return result ? result.code : code;
}
