/**
 * @neokapi/kapi-react — zero-config i18n for React.
 *
 * Usage:
 *   import neokapi from '@neokapi/kapi-react/vite';
 *   // or: from '@neokapi/kapi-react/webpack'
 *   // or: from '@neokapi/kapi-react/rollup'
 *   // or: from '@neokapi/kapi-react/esbuild'
 */

import { createUnplugin } from "unplugin";
import { transform } from "./transform.ts";
import { buildChunkManifest, type BundleLike } from "./chunk-manifest.ts";
import type { PluginOptions } from "../types.ts";

export type { PluginOptions };
export type { TranslationsManifest } from "./chunk-manifest.ts";

/**
 * Rollup hook context subset we use for asset emission. Narrow
 * interface so the plugin stays typable without importing Rollup.
 */
interface EmitContext {
  emitFile: (opts: { type: "asset"; fileName: string; source: string }) => void;
}

const MANIFEST_FILENAME = "translations-manifest.json";

export const unpluginFactory = (options: PluginOptions = {}) => {
  // module id → hashes the transform emitted into `__t`/`__tx` calls.
  // Consumed by the Vite/Rollup `generateBundle` hook to emit the
  // per-chunk manifest (issue #406). Only populated in runtime mode.
  const hashesByFile = new Map<string, Set<string>>();

  function emitManifest(ctx: EmitContext, bundle: BundleLike): void {
    if (options.mode !== "runtime") return;
    const manifest = buildChunkManifest(bundle, hashesByFile);
    ctx.emitFile({
      type: "asset",
      fileName: MANIFEST_FILENAME,
      source: JSON.stringify(manifest, null, 2),
    });
  }

  return {
    name: "neokapi-react",
    enforce: "pre" as const,

    buildStart() {
      // Full builds start clean. Dev-server HMR doesn't call this
      // per edit, but dev doesn't emit a manifest anyway (no bundle).
      hashesByFile.clear();
    },

    transformInclude(id: string) {
      return /\.[jt]sx$/.test(id);
    },

    transform(code: string, id: string) {
      // Dev mode: no locale and no runtime mode → no-op
      if (!options.locale && options.mode !== "runtime") return null;
      const result = transform(code, id, options);
      if (!result) return null;
      if (options.mode === "runtime" && result.hashes.length > 0) {
        hashesByFile.set(id, new Set(result.hashes));
      }
      return { code: result.code };
    },

    vite: {
      generateBundle(this: EmitContext, _opts: unknown, bundle: BundleLike) {
        emitManifest(this, bundle);
      },
    },
    rollup: {
      generateBundle(this: EmitContext, _opts: unknown, bundle: BundleLike) {
        emitManifest(this, bundle);
      },
    },
  };
};

export const unplugin = /* #__PURE__ */ createUnplugin(unpluginFactory);

export default unplugin;
