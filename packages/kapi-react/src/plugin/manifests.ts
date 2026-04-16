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

import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';

export type AutoManifest = {
  components: Record<string, string | null>;
  aliases?: Record<string, string>;
};

/**
 * Maps HTMLXxxElement type names to HTML tag names.
 */
const HTML_TYPE_TO_TAG: Record<string, string> = {
  Anchor: 'a',
  Button: 'button',
  Div: 'div',
  Form: 'form',
  Heading: 'h2',
  Image: 'img',
  Input: 'input',
  Label: 'label',
  LI: 'li',
  Nav: 'nav',
  OList: 'ol',
  Paragraph: 'p',
  Select: 'select',
  Span: 'span',
  UList: 'ul',
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
    if (source.startsWith('.') || source.startsWith('/')) continue;

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
    const libManifest = tryLoadJSON<AutoManifest>(
      join(packageDir, 'i18n-manifest.json'),
    );
    if (libManifest) return libManifest;
  }

  // Strategy 2: Community-maintained manifest
  if (communityManifestDir) {
    // Convert @radix-ui/react-tabs → radix-ui/react-tabs.json
    const normalizedName = packageName.replace(/^@/, '');
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
function resolvePackageDir(
  packageName: string,
  projectRoot: string,
): string | null {
  // Try direct node_modules path
  const directPath = join(projectRoot, 'node_modules', packageName);
  if (existsSync(join(directPath, 'package.json'))) {
    return directPath;
  }

  // Try require.resolve to handle pnpm, yarn PnP, etc.
  try {
    const entryPoint = require.resolve(`${packageName}/package.json`, {
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
  }>(join(packageDir, 'package.json'));

  if (!pkgJson) return null;

  // Resolve types file path
  let typesPath: string | null = null;
  if (pkgJson.types) {
    typesPath = join(packageDir, pkgJson.types);
  } else if (pkgJson.typings) {
    typesPath = join(packageDir, pkgJson.typings);
  } else if (pkgJson.exports) {
    // Check exports['.'].types
    const rootExport = pkgJson.exports['.'];
    if (rootExport && typeof rootExport === 'object' && rootExport.types) {
      typesPath = join(packageDir, rootExport.types);
    }
  }

  if (!typesPath || !existsSync(typesPath)) return null;

  let dtsContent: string;
  try {
    dtsContent = readFileSync(typesPath, 'utf-8');
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
  const aliasRegex =
    /export\s+(?:declare\s+)?(?:const|var|let)\s+(\w+)\s*:\s*typeof\s+(\w+)\s*;/g;
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
    const content = readFileSync(filePath, 'utf-8');
    return JSON.parse(content) as T;
  } catch {
    return null;
  }
}
