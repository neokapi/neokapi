// This file has been automatically migrated to valid ESM format by Storybook.
import type { StorybookConfig } from "@storybook/react-vite";
import tailwindcss from "@tailwindcss/vite";
import { dirname, join } from "path";
import { createRequire } from "module";

const require = createRequire(import.meta.url);

/**
 * Resolve package paths from this directory so Storybook finds packages
 * installed under packages/ui/node_modules in the npm workspace.
 */
function getAbsolutePath(value: string): string {
  return dirname(require.resolve(join(value, "package.json")));
}

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  addons: [getAbsolutePath("@storybook/addon-themes"), getAbsolutePath("@storybook/addon-docs")],
  framework: {
    name: getAbsolutePath("@storybook/react-vite") as "@storybook/react-vite",
    options: {},
  },
  viteFinal(config) {
    config.plugins = config.plugins || [];
    config.plugins.push(tailwindcss());
    // When building for GitHub Pages, serve from /storybook/ subpath.
    if (process.env.STORYBOOK_BASE_PATH) {
      config.base = process.env.STORYBOOK_BASE_PATH;
    }
    return config;
  },
};

export default config;
