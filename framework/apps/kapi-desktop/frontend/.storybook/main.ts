import type { StorybookConfig } from "@storybook/react-vite";
import tailwindcss from "@tailwindcss/vite";
import path from "path";
import { createRequire } from "module";

const require = createRequire(import.meta.url);
const __dirname = import.meta.dirname;

function getAbsolutePath(value: string): string {
  return path.dirname(require.resolve(path.join(value, "package.json")));
}

const config: StorybookConfig = {
  stories: [
    "../src/**/*.stories.@(ts|tsx)",
    "../../../../../packages/flow-editor/src/**/*.stories.@(ts|tsx)",
    "../../../../../packages/ui/src/**/*.stories.@(ts|tsx)",
  ],
  addons: [getAbsolutePath("@storybook/addon-themes"), getAbsolutePath("@storybook/addon-docs")],
  framework: {
    name: getAbsolutePath("@storybook/react-vite") as "@storybook/react-vite",
    options: {},
  },
  viteFinal: async (config) => {
    config.plugins = config.plugins || [];
    config.plugins.push(tailwindcss());

    config.resolve = config.resolve || {};
    config.resolve.alias = {
      ...config.resolve.alias,
      "@neokapi/ui-primitives": path.resolve(__dirname, "../../../../../packages/ui/src"),
      "@neokapi/flow-editor": path.resolve(__dirname, "../../../../../packages/flow-editor/src"),
    };

    return config;
  },
};

export default config;
