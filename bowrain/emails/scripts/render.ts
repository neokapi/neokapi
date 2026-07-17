/**
 * render.ts — pre-renders React Email templates to HTML files consumed by
 * the Go mailer package (platform/mailer/).
 *
 * English (the source locale) renders straight from src/ into
 * ../../mailer/templates/*.html. For every compiled catalog found in
 * translations/<locale>.json (produced by `make l10n-emails`: kapi-react
 * extraction → kapi pseudo-translate / TM recycle → kapi-react compile),
 * the template module is re-bundled with the @neokapi/kapi-react esbuild
 * plugin in inline mode — which bakes that locale's strings into the JSX —
 * and rendered into ../../mailer/templates/<locale>/*.html. The Go mailer
 * embeds the whole tree and picks the variant per recipient locale,
 * falling back to English.
 *
 * Run:  vp run build   (from bowrain/emails/)
 * Make: make email-build (en only) / make l10n-emails (all locales)
 */

import { build } from "esbuild";
import neokapi from "@neokapi/kapi-react/esbuild";
import { mkdirSync, writeFileSync, readdirSync, readFileSync, existsSync, rmSync } from "fs";
import { dirname, resolve } from "path";
import { fileURLToPath, pathToFileURL } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const pkgDir = resolve(__dirname, "..");

// Output directory: platform/mailer/templates/ (relative to this script)
const outDir = resolve(pkgDir, "../mailer/templates");

type RenderTemplates = () => Promise<Record<string, string>>;

function writeAll(dir: string, rendered: Record<string, string>): void {
  mkdirSync(dir, { recursive: true });
  for (const [name, html] of Object.entries(rendered)) {
    writeFileSync(resolve(dir, `${name}.html`), html, "utf-8");
    console.log(`✓  Rendered ${name}.html → ${dir}`);
  }
}

/** Locales with a compiled runtime catalog under translations/. */
function targetLocales(): string[] {
  const dir = resolve(pkgDir, "translations");
  if (!existsSync(dir)) return [];
  return readdirSync(dir)
    .filter((f) => f.endsWith(".json"))
    .map((f) => f.replace(/\.json$/, ""))
    .filter((l) => l !== "en")
    .sort();
}

/**
 * Bundles scripts/templates.tsx with the kapi-react inline transform for
 * the given locale and imports the result. The bundle keeps node_modules
 * imports external (it lives inside the package, so resolution works) —
 * only the template sources are transformed and inlined.
 */
async function localizedRenderer(locale: string): Promise<RenderTemplates> {
  // Same componentMap as extraction (kapi-react.config.json) so the
  // transform hashes blocks identically to the extract CLI.
  const config = JSON.parse(readFileSync(resolve(pkgDir, "kapi-react.config.json"), "utf-8")) as {
    componentMap: Record<string, string>;
  };
  const outfile = resolve(pkgDir, ".render", `templates-${locale}.mjs`);
  await build({
    entryPoints: [resolve(__dirname, "templates.tsx")],
    outfile,
    bundle: true,
    format: "esm",
    platform: "node",
    packages: "external",
    jsx: "automatic",
    plugins: [
      neokapi({
        mode: "inline",
        locale,
        translationsDir: resolve(pkgDir, "translations"),
        componentMap: config.componentMap,
      }),
    ],
  });
  const mod = (await import(pathToFileURL(outfile).href)) as {
    renderTemplates: RenderTemplates;
  };
  return mod.renderTemplates;
}

async function main(): Promise<void> {
  // ── English (source) ──────────────────────────────────────────────────
  const { renderTemplates } = (await import("./templates.js")) as {
    renderTemplates: RenderTemplates;
  };
  writeAll(outDir, await renderTemplates());

  // ── Localized variants (one subdirectory per compiled catalog) ────────
  for (const locale of targetLocales()) {
    const renderLocalized = await localizedRenderer(locale);
    writeAll(resolve(outDir, locale), await renderLocalized());
  }
  rmSync(resolve(pkgDir, ".render"), { recursive: true, force: true });

  console.log(`\nAll templates written to ${outDir}`);
}

main().catch((err: unknown) => {
  console.error("Template render failed:", err);
  process.exit(1);
});
