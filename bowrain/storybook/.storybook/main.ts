import { createMainConfig } from "@neokapi/storybook-config/main";

const config = createMainConfig(
  {
    stories: [
      // Kapi foundations — exclude editor/ stories that bowrain/packages/ui
      // also provides (TagPalette, InlineCodeLegend, InlinePreview,
      // TagValidationBar) to avoid duplicate story IDs.
      "../../../packages/ui/src/stories/!(editor)/**/*.stories.@(ts|tsx)",
      "../../../packages/ui/src/stories/*.stories.@(ts|tsx)",
      "../../../packages/flow-editor/src/**/*.stories.@(ts|tsx)",
      // Bowrain UI components (includes its own editor stories)
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
