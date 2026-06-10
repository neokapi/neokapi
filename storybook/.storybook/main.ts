import { createMainConfig } from "@neokapi/storybook-config/main";

const config = createMainConfig(
  {
    stories: [
      "../../packages/ui/src/**/*.stories.@(ts|tsx)",
      "../../packages/flow-editor/src/**/*.stories.@(ts|tsx)",
      "../../packages/kapi-lab/src/**/*.stories.@(ts|tsx)",
      "../../packages/docs-shared/src/**/*.stories.@(ts|tsx)",
      "../../apps/kapi-desktop/frontend/src/**/*.stories.@(ts|tsx)",
    ],
    i18n: true,
  },
  import.meta,
);

config.staticDirs = ["../../apps/kapi-desktop/frontend/public"];

export default config;
