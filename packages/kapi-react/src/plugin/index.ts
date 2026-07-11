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

/** Review mode: explicit option, or the KAPI_REVIEW=1 env toggle. */
function reviewEnabled(options: PluginOptions): boolean {
  if (options.review !== undefined) return options.review;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const env = (globalThis as any).process?.env?.KAPI_REVIEW;
  return env === "1" || env === "true";
}

/** Set once the dev server mounted the middleware — gates injection. */
let reviewDevServerActive = false;

const REVIEW_VIRTUAL_ID = "virtual:kapi-react-review";

export const unpluginFactory = (rawOptions: PluginOptions = {}) => {
  // Resolve the review toggle once (option or KAPI_REVIEW env) so the
  // transform, middleware, and html injection agree.
  const options: PluginOptions = { ...rawOptions, review: reviewEnabled(rawOptions) };
  if (options.review && options.mode !== "runtime") {
    console.warn(
      "[neokapi] review mode needs mode: 'runtime' (live dictionary lookups); " +
        "stamping data-kapi-* attributes anyway, but in-place repaint won't work.",
    );
  }
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

      // ── In-context review (dev server only) ──
      // Mounts the review middleware and injects the overlay client.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      async configureServer(server: any) {
        if (!reviewEnabled(options)) return;
        const { ReviewStore, handleReviewRequest } = await import("../review/store.ts");
        const store = new ReviewStore(options.reviewKlfDir ?? "i18n");
        const base = "/__kapi/review";
        server.middlewares.use(
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          (req: any, res: any, next: () => void) => {
            const url: string = req.url ?? "";
            if (!url.startsWith(base)) return next();
            const handled = handleReviewRequest(store, req, res, url.slice(base.length));
            if (!handled) next();
          },
        );
        reviewDevServerActive = true;
      },

      // The overlay bootstrap lives in a virtual module so the bare
      // `@neokapi/kapi-react/review` import resolves through Vite's
      // pipeline — an inline script tag would hit the browser's
      // native resolver and fail on the bare specifier.
      resolveId(id: string) {
        if (id === REVIEW_VIRTUAL_ID) return "\0" + REVIEW_VIRTUAL_ID;
        return null;
      },
      load(id: string) {
        if (id === "\0" + REVIEW_VIRTUAL_ID) {
          return "import { initKapiReview } from '@neokapi/kapi-react/review';\ninitKapiReview();\n";
        }
        return null;
      },

      transformIndexHtml(html: string) {
        if (!reviewEnabled(options) || !reviewDevServerActive) return html;
        return {
          html,
          tags: [
            {
              tag: "script",
              attrs: { type: "module", src: `/@id/__x00__${REVIEW_VIRTUAL_ID}` },
              injectTo: "body" as const,
            },
          ],
        };
      },
    },
    rollup: {
      generateBundle(this: EmitContext, _opts: unknown, bundle: BundleLike) {
        emitManifest(this, bundle);
      },
    },

    // Webpack 5 / Rspack: rebuild the chunk → module-resource mapping
    // from the chunk graph and emit the same manifest asset the
    // Vite/Rollup path produces. Without this, `kapi-react split` had
    // nothing to consume under the exact stacks the Next.js docs
    // steer users to, and route-level lazy loading silently no-oped.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    webpack(compiler: any) {
      if (options.mode !== "runtime") return;
      const { webpack } = compiler;
      compiler.hooks.thisCompilation.tap("neokapi-react", (compilation: any) => {
        compilation.hooks.processAssets.tap(
          {
            name: "neokapi-react",
            stage: webpack.Compilation.PROCESS_ASSETS_STAGE_SUMMARIZE,
          },
          () => {
            const bundle: BundleLike = {};
            for (const chunk of compilation.chunks) {
              const modules: Record<string, unknown> = {};
              for (const mod of compilation.chunkGraph.getChunkModulesIterable(chunk)) {
                const resource = (mod as { resource?: string }).resource;
                if (resource) modules[resource] = true;
              }
              const name: string = chunk.name ?? chunk.id?.toString() ?? "chunk";
              bundle[name] = { type: "chunk", name, modules };
            }
            const manifest = buildChunkManifest(bundle, hashesByFile);
            compilation.emitAsset(
              MANIFEST_FILENAME,
              new webpack.sources.RawSource(JSON.stringify(manifest, null, 2)),
            );
          },
        );
      });
    },

    // esbuild: with a metafile, map outputs → inputs for real
    // per-chunk manifests; without one, fall back to a single-chunk
    // manifest (better than silently emitting nothing) and say so.
    esbuild: {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      setup(build: any) {
        if (options.mode !== "runtime") return;
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        build.onEnd(async (result: any) => {
          const outdir = build.initialOptions.outdir;
          if (!outdir) return;
          const bundle: BundleLike = {};
          if (result.metafile) {
            for (const [outFile, output] of Object.entries(
              result.metafile.outputs as Record<string, { inputs: Record<string, unknown> }>,
            )) {
              if (!outFile.endsWith(".js") && !outFile.endsWith(".mjs")) continue;
              const modules: Record<string, unknown> = {};
              for (const input of Object.keys(output.inputs)) {
                modules[input] = true;
                // hashesByFile keys are absolute; metafile inputs are
                // outdir-relative — index both spellings.
                modules[resolvePath(input)] = true;
              }
              const name = basenameNoExt(outFile);
              bundle[name] = { type: "chunk", name, modules };
            }
          } else {
            const modules: Record<string, unknown> = {};
            for (const id of hashesByFile.keys()) modules[id] = true;
            bundle["main"] = { type: "chunk", name: "main", modules };
            console.warn(
              "[neokapi] esbuild build has no metafile — emitting a single-chunk " +
                "translations-manifest.json. Pass `metafile: true` for per-chunk splitting.",
            );
          }
          const manifest = buildChunkManifest(bundle, hashesByFile);
          const { writeFile, mkdir } = await import("node:fs/promises");
          await mkdir(outdir, { recursive: true });
          await writeFile(
            joinPath(outdir, MANIFEST_FILENAME),
            JSON.stringify(manifest, null, 2),
            "utf8",
          );
        });
      },
    },
  };
};

// Tiny path helpers — avoid a static node:path import in a module
// that also loads in non-Node bundler sandboxes.
function joinPath(dir: string, file: string): string {
  return dir.endsWith("/") ? dir + file : `${dir}/${file}`;
}

function basenameNoExt(p: string): string {
  const base = p.slice(p.lastIndexOf("/") + 1);
  const dot = base.lastIndexOf(".");
  return dot > 0 ? base.slice(0, dot) : base;
}

function resolvePath(p: string): string {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const proc = (globalThis as any).process;
  const cwd: string = proc?.cwd?.() ?? "";
  if (!cwd || p.startsWith("/") || /^[A-Za-z]:/.test(p)) return p;
  return joinPath(cwd, p);
}

export const unplugin = /* #__PURE__ */ createUnplugin(unpluginFactory);

export default unplugin;
