/**
 * Resolves component → HTML element mappings from library i18n manifests.
 *
 * Resolution order for a package like "@radix-ui/react-tabs":
 *   1. <package>/i18n-manifest.json          (library ships it)
 *   2. @nkzw/i18n-manifests/radix-ui/react-tabs.json  (community-maintained)
 *   3. Auto-generated from .d.ts RefAttributes<HTMLElement> types
 *
 * Manifest format:
 *   {
 *     "components": {
 *       "Tabs":        "div",
 *       "TabsList":    "div",
 *       "TabsTrigger": "button",
 *       "TabsContent": "div"
 *     },
 *     "aliases": {
 *       "Root": "Tabs",
 *       "List": "TabsList",
 *       "Trigger": "TabsTrigger",
 *       "Content": "TabsContent"
 *     }
 *   }
 */

import type { Module } from "@swc/core";
import { existsSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";

const requireFromCwd = createRequire(join(process.cwd(), "__placeholder__"));

export type AutoManifest = {
  components: Record<string, string | null>;
  aliases?: Record<string, string>;
};

/**
 * Maps HTMLXxxElement type names to HTML tag names.
 */
const HTML_TYPE_TO_TAG: Record<string, string> = {
  Anchor: "a",
  Button: "button",
  Div: "div",
  Form: "form",
  Heading: "h2",
  Image: "img",
  Input: "input",
  Label: "label",
  LI: "li",
  Nav: "nav",
  OList: "ol",
  Paragraph: "p",
  Select: "select",
  Span: "span",
  UList: "ul",
};

/**
 * Given a set of import sources used in a file, resolve library manifests
 * and return a merged componentMap.
 *
 * @param importSources Map of import source → Map of importedName → localName
 *   e.g. { "@radix-ui/react-tabs": Map { "Root" → "Tabs", "Trigger" → "TabsTrigger" } }
 * @param projectRoot The root directory to resolve node_modules from
 * @param communityManifestDir Optional path to community manifest directory
 */
export function resolveLibraryManifests(
  importSources: Map<string, Map<string, string>>,
  projectRoot: string,
  communityManifestDir?: string,
): Record<string, string> {
  const result: Record<string, string> = {};

  for (const [source, importedNames] of importSources) {
    // Skip relative imports (handled by same-file deduction)
    if (source.startsWith(".") || source.startsWith("/")) continue;

    const manifest = loadManifest(source, projectRoot, communityManifestDir);
    if (!manifest) continue;

    // Build alias → canonical name lookup
    const aliasToCanonical = new Map<string, string>();
    if (manifest.aliases) {
      for (const [alias, canonical] of Object.entries(manifest.aliases)) {
        aliasToCanonical.set(alias, canonical);
      }
    }

    // For each imported name, resolve to HTML element
    for (const [importedName, localName] of importedNames) {
      // Try direct match first, then alias resolution
      const canonicalName = aliasToCanonical.get(importedName) || importedName;
      const htmlElement = manifest.components[canonicalName];

      if (htmlElement) {
        result[localName] = htmlElement;
      }
      // null means explicitly "don't translate" — we skip it
    }
  }

  return result;
}

/**
 * Load a manifest for a given package, trying multiple resolution strategies.
 */
function loadManifest(
  packageName: string,
  projectRoot: string,
  communityManifestDir?: string,
): AutoManifest | null {
  // Strategy 1: Library ships its own manifest
  const packageDir = resolvePackageDir(packageName, projectRoot);
  if (packageDir) {
    const libManifest = tryLoadJSON<AutoManifest>(join(packageDir, "i18n-manifest.json"));
    if (libManifest) return libManifest;
  }

  // Strategy 2: Community-maintained manifest
  if (communityManifestDir) {
    // Convert @radix-ui/react-tabs → radix-ui/react-tabs.json
    const normalizedName = packageName.replace(/^@/, "");
    const communityPath = join(communityManifestDir, `${normalizedName}.json`);
    const communityManifest = tryLoadJSON<AutoManifest>(communityPath);
    if (communityManifest) return communityManifest;
  }

  // Strategy 3: Auto-generate from .d.ts types
  if (packageDir) {
    return autoGenerateManifest(packageDir);
  }

  return null;
}

/**
 * Resolve the directory of an installed package.
 */
function resolvePackageDir(packageName: string, projectRoot: string): string | null {
  // Try direct node_modules path
  const directPath = join(projectRoot, "node_modules", packageName);
  if (existsSync(join(directPath, "package.json"))) {
    return directPath;
  }

  // Walk up ancestor node_modules directories (npm/yarn workspaces
  // often hoist deps to the repo root instead of the sub-package).
  let dir = projectRoot;
  while (true) {
    const candidate = join(dir, "node_modules", packageName, "package.json");
    if (existsSync(candidate)) return dirname(candidate);
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }

  // Try require.resolve as a last resort (handles pnpm hoist,
  // yarn PnP, etc). ESM-safe via createRequire.
  try {
    const entryPoint = requireFromCwd.resolve(`${packageName}/package.json`, {
      paths: [projectRoot],
    });
    return dirname(entryPoint);
  } catch {
    return null;
  }
}

/**
 * Auto-generate a manifest by parsing .d.ts files for RefAttributes<HTMLElement>.
 */
function autoGenerateManifest(packageDir: string): AutoManifest | null {
  // Find the types entry point
  const pkgJson = tryLoadJSON<{
    types?: string;
    typings?: string;
    exports?: Record<string, any>;
  }>(join(packageDir, "package.json"));

  if (!pkgJson) return null;

  // Resolve types file path
  let typesPath: string | null = null;
  if (pkgJson.types) {
    typesPath = join(packageDir, pkgJson.types);
  } else if (pkgJson.typings) {
    typesPath = join(packageDir, pkgJson.typings);
  } else if (pkgJson.exports) {
    // Check exports['.'].types
    const rootExport = pkgJson.exports["."];
    if (rootExport && typeof rootExport === "object" && rootExport.types) {
      typesPath = join(packageDir, rootExport.types);
    }
  }

  if (!typesPath || !existsSync(typesPath)) return null;

  let dtsContent: string;
  try {
    dtsContent = readFileSync(typesPath, "utf-8");
  } catch {
    return null;
  }

  return parseManifestFromDTS(dtsContent);
}

/**
 * Parse a .d.ts file to extract component → HTML element mappings.
 *
 * Looks for patterns like:
 *   export declare const TabsTrigger: React.ForwardRefExoticComponent<
 *     TabsTriggerProps & React.RefAttributes<HTMLButtonElement>
 *   >;
 */
export function parseManifestFromDTS(dtsContent: string): AutoManifest | null {
  const components: Record<string, string | null> = {};
  const aliases: Record<string, string> = {};

  // Match exported component declarations with RefAttributes
  const exportDeclRegex =
    /export\s+(?:declare\s+)?(?:const|var|let)\s+(\w+)\s*:\s*[^;]*?RefAttributes\s*<\s*HTML(\w+)Element\s*>/g;

  let match;
  while ((match = exportDeclRegex.exec(dtsContent)) !== null) {
    const [, componentName, htmlTypeName] = match;
    const htmlTag = HTML_TYPE_TO_TAG[htmlTypeName];
    if (htmlTag) {
      components[componentName] = htmlTag;
    }
  }

  // Match type alias re-exports: export declare const Root: typeof Tabs;
  const aliasRegex = /export\s+(?:declare\s+)?(?:const|var|let)\s+(\w+)\s*:\s*typeof\s+(\w+)\s*;/g;
  while ((match = aliasRegex.exec(dtsContent)) !== null) {
    const [, alias, canonical] = match;
    if (alias !== canonical && components[canonical]) {
      aliases[alias] = canonical;
    }
  }

  if (Object.keys(components).length === 0) return null;

  return { components, aliases };
}

function tryLoadJSON<T>(filePath: string): T | null {
  try {
    if (!existsSync(filePath)) return null;
    const content = readFileSync(filePath, "utf-8");
    return JSON.parse(content) as T;
  } catch {
    return null;
  }
}

// ─── Import scanning ─────────────────────────────────────────

/**
 * Collects every non-relative import in a parsed module, grouped by
 * source. Used by `resolveLibraryManifests` to decide which library
 * manifests (or .d.ts files) to consult.
 *
 * Returned shape: `source → importedName → localName`. The imported
 * name is the library's own export name (`Trigger`); the local
 * name is what the file uses (`TabsTrigger` after aliasing).
 */
export function collectImports(mod: Module): Map<string, Map<string, string>> {
  const out = new Map<string, Map<string, string>>();
  for (const item of mod.body) {
    if (item.type !== "ImportDeclaration") continue;
    const source = item.source.value;
    if (!source || source.startsWith(".") || source.startsWith("/")) continue;
    let names = out.get(source);
    if (!names) {
      names = new Map();
      out.set(source, names);
    }
    for (const spec of item.specifiers) {
      if (spec.type !== "ImportSpecifier") continue;
      const imported = spec.imported?.value ?? spec.local.value;
      names.set(imported, spec.local.value);
    }
  }
  return out;
}

// ─── Per-build cache ─────────────────────────────────────────

/**
 * Build-scoped cache of resolved library → componentMap slices. The
 * key is `projectRoot|communityManifestDir` so simultaneous plugin
 * runs with different roots don't cross-contaminate. Cache lives for
 * the life of the Node process, so a long-running dev server shares
 * it across reloads.
 */
const libraryMapCache = new Map<string, Map<string, Record<string, string>>>();

/**
 * Resolves library manifests for a module's imports, memoising per
 * `(projectRoot, communityManifestDir, source)` triplet so that a
 * 500-file Vite build parses each library's .d.ts at most once.
 *
 * When `filename` is provided, also loads the manifest of the package
 * that owns the file being transformed. This lets a library's own
 * source files benefit from its manifest — e.g. `packages/ui/src/
 * components/ui/dialog.tsx` renders `<Button>` via a relative
 * import, which the library-manifest resolver otherwise skips.
 * Without this, every consumer would have to repeat the library's
 * own componentMap in their local plugin config.
 */
export function resolveLibraryComponentMap(
  mod: Module,
  projectRoot: string,
  communityManifestDir?: string,
  filename?: string,
): Record<string, string> {
  const cacheKey = `${projectRoot}|${communityManifestDir ?? ""}`;
  let perSource = libraryMapCache.get(cacheKey);
  if (!perSource) {
    perSource = new Map();
    libraryMapCache.set(cacheKey, perSource);
  }

  const merged: Record<string, string> = {};
  const imports = collectImports(mod);
  for (const [source, names] of imports) {
    let sourceMap = perSource.get(source);
    if (!sourceMap) {
      sourceMap = resolveSingleLibrary(source, projectRoot, communityManifestDir);
      perSource.set(source, sourceMap);
    }
    // Remap the library's canonical names to the file's local
    // identifiers. e.g. `import { Trigger as TabsTrigger } …`
    // needs `TabsTrigger → button` in the merged map.
    for (const [importedName, localName] of names) {
      const htmlTag = sourceMap[importedName];
      if (htmlTag) merged[localName] = htmlTag;
    }
  }

  // Own-package manifest: if the file being compiled belongs to a
  // package that ships `i18n-manifest.json`, apply that manifest to
  // the merged map. Covers relative imports within the library's own
  // source tree where the import-based resolution above doesn't
  // fire. Merge order: own-package entries fill gaps left by
  // imports, but don't override explicit import-based resolutions
  // (the library's declared mapping for its own exports should
  // already be authoritative, so either path gives the same answer).
  if (filename) {
    const ownPackageMap = resolveOwnPackageManifest(filename);
    for (const [name, tag] of Object.entries(ownPackageMap)) {
      if (merged[name] === undefined) merged[name] = tag;
    }
  }

  return merged;
}

/**
 * Cache of package root → manifest map. Lookup key is the resolved
 * package directory (not the filename) so every file in the same
 * package hits the cache after the first load.
 */
const ownPackageCache = new Map<string, Record<string, string>>();
/** Cache of filename → owning package dir (or null). */
const fileOwningPackageCache = new Map<string, string | null>();

/**
 * For a given file, locate its owning package (nearest package.json
 * walking up) and return the flat componentMap its manifest declares.
 * Aliases are inlined so the caller can treat the result like any
 * other componentMap.
 */
function resolveOwnPackageManifest(filename: string): Record<string, string> {
  const packageDir = findOwningPackageDir(filename);
  if (!packageDir) return {};
  const cached = ownPackageCache.get(packageDir);
  if (cached) return cached;

  const manifest = tryLoadJSON<AutoManifest>(join(packageDir, "i18n-manifest.json"));
  const out: Record<string, string> = {};
  if (manifest) {
    for (const [name, tag] of Object.entries(manifest.components)) {
      if (tag) out[name] = tag;
    }
    if (manifest.aliases) {
      for (const [alias, canonical] of Object.entries(manifest.aliases)) {
        const tag = manifest.components[canonical];
        if (tag) out[alias] = tag;
      }
    }
  }
  ownPackageCache.set(packageDir, out);
  return out;
}

function findOwningPackageDir(filename: string): string | null {
  const cached = fileOwningPackageCache.get(filename);
  if (cached !== undefined) return cached;

  let dir = dirname(filename);
  while (true) {
    if (existsSync(join(dir, "package.json"))) {
      fileOwningPackageCache.set(filename, dir);
      return dir;
    }
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  fileOwningPackageCache.set(filename, null);
  return null;
}

function resolveSingleLibrary(
  source: string,
  projectRoot: string,
  communityManifestDir?: string,
): Record<string, string> {
  const single = new Map<string, Map<string, string>>();
  single.set(source, new Map());
  const manifest = loadManifest(source, projectRoot, communityManifestDir);
  if (!manifest) return {};

  const out: Record<string, string> = {};
  const aliasToCanonical = new Map<string, string>();
  if (manifest.aliases) {
    for (const [alias, canonical] of Object.entries(manifest.aliases)) {
      aliasToCanonical.set(alias, canonical);
    }
  }
  for (const [name, tag] of Object.entries(manifest.components)) {
    if (tag) out[name] = tag;
  }
  for (const [alias, canonical] of aliasToCanonical) {
    if (manifest.components[canonical]) {
      out[alias] = manifest.components[canonical] as string;
    }
  }
  return out;
}
