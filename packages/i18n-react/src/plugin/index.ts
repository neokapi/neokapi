/**
 * @neokapi/i18n-react — zero-config i18n for React.
 *
 * Usage:
 *   import neokapi from '@neokapi/i18n-react/vite';
 *   // or: from '@neokapi/i18n-react/webpack'
 *   // or: from '@neokapi/i18n-react/rollup'
 *   // or: from '@neokapi/i18n-react/esbuild'
 */

import { createUnplugin } from "unplugin";
import { transform } from "./transform.ts";
import { buildChunkManifest, type BundleLike } from "./chunk-manifest.ts";
import type { PluginOptions } from "../types.ts";
import type { ReviewManifest, ReviewManifestEntry } from "../review/manifest.ts";

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

/**
 * Read-only review manifest emitted when `review: true`, in any render
 * mode (inline/SSG or runtime). Path matches the hosted overlay's
 * default fetch URL (`<base>translations/review.json`), so a static
 * build is self-sufficient: stamps + this file → `initKapiReviewHosted`
 * works with no dev server and no app i18n runtime (AD-035). Richer
 * manifests (all locales, term/check annotations) still come from
 * `neokapi-i18n compile --review`, which reads the whole KBF tree.
 */
const REVIEW_FILENAME = "translations/review.json";

/** Review mode: explicit option, or the KAPI_REVIEW=1 env toggle. */
function reviewEnabled(options: PluginOptions): boolean {
  if (options.review !== undefined) return options.review;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const env = (globalThis as any).process?.env?.KAPI_REVIEW;
  return env === "1" || env === "true";
}

/** Set once the dev server mounted the middleware — gates injection. */
let reviewDevServerActive = false;

const REVIEW_VIRTUAL_ID = "virtual:i18n-react-review";

export const unpluginFactory = (rawOptions: PluginOptions = {}) => {
  // Resolve the review toggle once (option or KAPI_REVIEW env) so the
  // transform, middleware, and html injection agree.
  const options: PluginOptions = { ...rawOptions, review: reviewEnabled(rawOptions) };
  // module id → hashes the transform emitted into `__t`/`__tx` calls.
  // Consumed by the Vite/Rollup `generateBundle` hook to emit the
  // per-chunk manifest (issue #406). Only populated in runtime mode.
  const hashesByFile = new Map<string, Set<string>>();
  // block hash → review data, unioned across every transformed file.
  // Serialized to `translations/review.json` when `review: true`.
  // Render-mode-independent (populated in inline and runtime alike).
  const reviewByHash = new Map<string, ReviewManifestEntry>();

  /** Merge one file's review entries into the build-wide manifest. */
  function mergeReview(fileManifest: ReviewManifest): void {
    for (const [hash, entry] of Object.entries(fileManifest)) {
      const existing = reviewByHash.get(hash);
      if (!existing) {
        reviewByHash.set(hash, entry);
        continue;
      }
      if (!existing.source && entry.source) existing.source = entry.source;
      if (!existing.properties.file && entry.properties.file)
        existing.properties = entry.properties;
      for (const [loc, text] of Object.entries(entry.targets)) existing.targets[loc] ??= text;
    }
  }

  /** Serialize the accumulated review manifest (sorted for reproducibility). */
  function buildReviewJSON(): string {
    const manifest: ReviewManifest = {};
    for (const hash of Array.from(reviewByHash.keys()).sort()) {
      manifest[hash] = reviewByHash.get(hash) as ReviewManifestEntry;
    }
    return JSON.stringify(manifest, null, 2);
  }

  function emitManifest(ctx: EmitContext, bundle: BundleLike): void {
    if (options.mode === "runtime") {
      const manifest = buildChunkManifest(bundle, hashesByFile);
      ctx.emitFile({
        type: "asset",
        fileName: MANIFEST_FILENAME,
        source: JSON.stringify(manifest, null, 2),
      });
    }
    // Review manifest: gated on `review`, not mode — a static/inline
    // build emits it too, so the hosted overlay works on SSG pages.
    if (options.review) {
      ctx.emitFile({ type: "asset", fileName: REVIEW_FILENAME, source: buildReviewJSON() });
    }
  }

  return {
    name: "neokapi-i18n",
    enforce: "pre" as const,

    buildStart() {
      // Full builds start clean. Dev-server HMR doesn't call this
      // per edit, but dev doesn't emit a manifest anyway (no bundle).
      hashesByFile.clear();
      reviewByHash.clear();
    },

    transformInclude(id: string) {
      return /\.[jt]sx$/.test(id);
    },

    transform(code: string, id: string) {
      // No-op unless there's work: a locale (inline), runtime mode, or
      // review (which stamps + records a manifest in any mode).
      if (!options.locale && options.mode !== "runtime" && !options.review) return null;
      const result = transform(code, id, options);
      if (!result) return null;
      if (options.mode === "runtime" && result.hashes.length > 0) {
        hashesByFile.set(id, new Set(result.hashes));
      }
      if (options.review) mergeReview(result.review);
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
        const store = new ReviewStore(options.reviewKbfDir ?? "i18n");
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
      // `@neokapi/i18n-react/review` import resolves through Vite's
      // pipeline — an inline script tag would hit the browser's
      // native resolver and fail on the bare specifier.
      resolveId(id: string) {
        if (id === REVIEW_VIRTUAL_ID) return "\0" + REVIEW_VIRTUAL_ID;
        return null;
      },
      load(id: string) {
        if (id === "\0" + REVIEW_VIRTUAL_ID) {
          return "import { initKapiReview } from '@neokapi/i18n-react/review';\ninitKapiReview();\n";
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
    // Vite/Rollup path produces. Without this, `neokapi-i18n split` had
    // nothing to consume under the exact stacks the Next.js docs
    // steer users to, and route-level lazy loading silently no-oped.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    webpack(compiler: any) {
      if (options.mode !== "runtime" && !options.review) return;
      const { webpack } = compiler;
      compiler.hooks.thisCompilation.tap("neokapi-i18n", (compilation: any) => {
        compilation.hooks.processAssets.tap(
          {
            name: "neokapi-i18n",
            stage: webpack.Compilation.PROCESS_ASSETS_STAGE_SUMMARIZE,
          },
          () => {
            if (options.mode === "runtime") {
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
            }
            if (options.review) {
              compilation.emitAsset(
                REVIEW_FILENAME,
                new webpack.sources.RawSource(buildReviewJSON()),
              );
            }
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
        if (options.mode !== "runtime" && !options.review) return;
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        build.onEnd(async (result: any) => {
          const outdir = build.initialOptions.outdir;
          if (!outdir) return;
          const { writeFile, mkdir } = await import("node:fs/promises");
          await mkdir(outdir, { recursive: true });

          if (options.mode === "runtime") {
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
            await writeFile(
              joinPath(outdir, MANIFEST_FILENAME),
              JSON.stringify(manifest, null, 2),
              "utf8",
            );
          }

          if (options.review) {
            await mkdir(joinPath(outdir, "translations"), { recursive: true });
            await writeFile(joinPath(outdir, REVIEW_FILENAME), buildReviewJSON(), "utf8");
          }
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
