import { createMainConfig } from "@neokapi/storybook-config/main";

const config = createMainConfig(
  {
    stories: [
      "../../packages/ui/src/**/*.stories.@(ts|tsx)",
      "../../packages/flow-editor/src/**/*.stories.@(ts|tsx)",
      "../../apps/kapi-desktop/frontend/src/**/*.stories.@(ts|tsx)",
      "../../kapi/apps/kapi-web/src/**/*.stories.@(ts|tsx)",
    ],
  },
  import.meta,
);

export default config;
