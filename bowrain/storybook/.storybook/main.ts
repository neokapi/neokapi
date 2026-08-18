import { createMainConfig } from "@neokapi/storybook-config/main";

const config = createMainConfig(
  {
    stories: [
      // Kapi foundations — exclude stories that bowrain/packages/ui also
      // provides, to avoid duplicate story IDs: the editor/ group (TagPalette,
      // InlineCodeLegend, InlinePreview, TagValidationBar) and ui/ErrorNotice
      // (bowrain has its own under packages/ui/src/errors).
      "../../../packages/ui/src/stories/!(editor|ui)/**/*.stories.@(ts|tsx)",
      "../../../packages/ui/src/stories/ui/!(ErrorNotice).stories.@(ts|tsx)",
      "../../../packages/ui/src/stories/*.stories.@(ts|tsx)",
      "../../../packages/flow-editor/src/**/*.stories.@(ts|tsx)",
      "../../../packages/editor-grid/src/**/*.stories.@(ts|tsx)",
      "../../../packages/status-views/src/**/*.stories.@(ts|tsx)",
      // Bowrain UI components (includes its own editor stories)
      "../../packages/ui/src/**/*.stories.@(ts|tsx)",
      // Bowrain apps
      "../../emails/src/**/*.stories.@(ts|tsx)",
      "../../apps/keycloak-theme/src/**/*.stories.@(ts|tsx)",
      // Shared Bowrain app shell (auth + locale-demand stories moved here).
      "../../packages/app/src/**/*.stories.@(ts|tsx)",
      "../../apps/bowrain/frontend/src/**/*.stories.@(ts|tsx)",
      "../../apps/ctrl/src/**/*.stories.@(ts|tsx)",
    ],
    i18n: true,
  },
  import.meta,
);

// The web shell's committed catalogs, so a story can be read in a target
// language: the locale toolbar fetches `<base>/translations/<locale>.json`, and
// without a static root there is nothing at that path to fetch.
config.staticDirs = ["../../apps/web/public"];

export default config;
