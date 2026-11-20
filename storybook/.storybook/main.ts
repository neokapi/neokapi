import { createMainConfig } from "@neokapi/storybook-config/main";
import { generateQpsCatalog } from "../../scripts/vite-qps-catalog.mjs";

const config = createMainConfig(
  {
    stories: [
      "../../web/src/components/KapiPlayground/PlaygroundDialog.stories.tsx",
      "../../web/src/components/Lab/LabLaunch.stories.tsx",
      "../../packages/ui/src/**/*.stories.@(ts|tsx)",
      "../../packages/flow-editor/src/**/*.stories.@(ts|tsx)",
      "../../packages/editor-grid/src/**/*.stories.@(ts|tsx)",
      "../../packages/status-views/src/**/*.stories.@(ts|tsx)",
      "../../packages/concept-ui/src/**/*.stories.@(ts|tsx)",
      "../../packages/context-explorer/src/**/*.stories.@(ts|tsx)",
      "../../packages/context-atlas/src/**/*.stories.@(ts|tsx)",
      "../../packages/project-screens/src/**/*.stories.@(ts|tsx)",
      "../../packages/kapi-lab/src/**/*.stories.@(ts|tsx)",
      "../../packages/docs-shared/src/**/*.stories.@(ts|tsx)",
      "../../apps/kapi-desktop/frontend/src/**/*.stories.@(ts|tsx)",
    ],
    i18n: true,
  },
  import.meta,
);

const sharedViteFinal = config.viteFinal;
config.viteFinal = async (viteConfig, options) => {
  // The desktop qps dictionary under staticDirs is build output (make kapi-desktop-qps).
  generateQpsCatalog("kapi-desktop-qps");
  const configured = sharedViteFinal ? await sharedViteFinal(viteConfig, options) : viteConfig;
  // The website preserves JSX for Docusaurus; Storybook compiles those components itself.
  configured.oxc = { ...configured.oxc, jsx: { runtime: "automatic" } };
  return configured;
};

config.staticDirs = ["../../apps/kapi-desktop/frontend/public"];

export default config;
