import type { StorybookConfig } from "@storybook/react-vite";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

function getAbsolutePath(value: string): string {
  return new URL(value, import.meta.url).pathname;
}

const config: StorybookConfig = {
  stories: [
    // Kapi Desktop page components
    "../src/**/*.stories.@(ts|tsx)",
    // Shared flow editor components
    "../../../../packages/flow-editor/src/**/*.stories.@(ts|tsx)",
  ],
  addons: [
    getAbsolutePath("@storybook/addon-themes"),
    getAbsolutePath("@storybook/addon-docs"),
  ],
  framework: getAbsolutePath("@storybook/react-vite"),
  viteFinal: async (config) => {
    config.plugins = config.plugins || [];
    config.plugins.push(tailwindcss());

    // Resolve shared packages for storybook
    config.resolve = config.resolve || {};
    config.resolve.alias = {
      ...config.resolve.alias,
      "@neokapi/ui-primitives": path.resolve(
        import.meta.dirname,
        "../../../../packages/ui/src",
      ),
      "@neokapi/flow-editor": path.resolve(
        import.meta.dirname,
        "../../../../packages/flow-editor/src",
      ),
    };

    return config;
  },
};

export default config;
