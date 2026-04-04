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
    viteFinal(config) {
      config.plugins = config.plugins || [];
      config.plugins.push(tailwindcss());

      const envBasePath = options.basePath || process.env.STORYBOOK_BASE_PATH;
      if (envBasePath) {
        config.base = envBasePath;
      }
      return config;
    },
  };
}
