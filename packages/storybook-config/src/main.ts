import type { StorybookConfig } from "@storybook/react-vite";
import tailwindcss from "@tailwindcss/vite";
import { dirname, join } from "path";
import { createRequire } from "module";

/**
 * Resolve package paths so Storybook finds packages installed in
 * the npm workspace node_modules.
 */
function getAbsolutePath(value: string, requireFn: NodeRequire): string {
  return dirname(requireFn.resolve(join(value, "package.json")));
}

export interface CreateMainConfigOptions {
  /** Story glob patterns (relative to the Storybook config directory). */
  stories: string[];
  /** Optional base path for GitHub Pages deployment (e.g. "/storybook/"). */
  basePath?: string;
  /**
   * Enable @neokapi/react runtime-mode transform so stories pick up
   * the locale toolbar. Pair with `i18n` in createPreview().
   *
   * Pass `true` to transform every .tsx in the build graph, or pass an
   * array of absolute path prefixes to scope the transform — useful for
   * skipping workspace deps that hit upstream transform bugs.
   */
  i18n?: boolean | { include: string[] };
}

/**
 * Creates a Storybook main config with shared defaults.
 * Each product Storybook calls this with its own story globs.
 */
export function createMainConfig(
  options: CreateMainConfigOptions,
  importMeta: ImportMeta,
): StorybookConfig {
  const req = createRequire(importMeta.url);

  return {
    stories: options.stories,
    addons: [
      getAbsolutePath("@storybook/addon-themes", req),
      getAbsolutePath("@storybook/addon-docs", req),
    ],
    framework: {
      name: getAbsolutePath("@storybook/react-vite", req) as "@storybook/react-vite",
      options: {},
    },
    async viteFinal(config) {
      config.plugins = config.plugins || [];
      config.plugins.push(tailwindcss());

      if (options.i18n) {
        const neokapi = (await import("@neokapi/react/vite")).default;
        const raw = neokapi({ mode: "runtime" }) as {
          name: string;
          transform?: (this: unknown, code: string, id: string) => unknown;
        };
        if (typeof options.i18n === "object" && options.i18n.include) {
          const includes = options.i18n.include;
          const rawTransform = raw.transform;
          config.plugins.push({
            ...raw,
            transform(code: string, id: string) {
              if (!includes.some((p) => id.startsWith(p))) return null;
              return rawTransform ? rawTransform.call(this, code, id) : null;
            },
          });
        } else {
          config.plugins.push(raw);
        }
      }

      const envBasePath = options.basePath || process.env.STORYBOOK_BASE_PATH;
      if (envBasePath) {
        config.base = envBasePath;
      }
      return config;
    },
  };
}
