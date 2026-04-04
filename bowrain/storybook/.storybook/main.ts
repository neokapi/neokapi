import { createMainConfig } from "@neokapi/storybook-config/main";

const config = createMainConfig(
  {
    stories: [
      // Kapi foundations
      "../../../packages/ui/src/**/*.stories.@(ts|tsx)",
      "../../../packages/flow-editor/src/**/*.stories.@(ts|tsx)",
      // Bowrain UI components
      "../../packages/ui/src/**/*.stories.@(ts|tsx)",
      // Bowrain apps
      "../../emails/src/**/*.stories.@(ts|tsx)",
      "../../apps/keycloak-theme/src/**/*.stories.@(ts|tsx)",
      "../../apps/web/src/auth/**/*.stories.@(ts|tsx)",
      "../../apps/bowrain/frontend/src/**/*.stories.@(ts|tsx)",
    ],
  },
  import.meta,
);

export default config;
