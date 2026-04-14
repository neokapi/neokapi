import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createMainConfig } from "@neokapi/storybook-config/main";

const here = resolve(fileURLToPath(import.meta.url), "..");

const config = createMainConfig(
  {
    stories: [
      "../../packages/ui/src/**/*.stories.@(ts|tsx)",
      "../../packages/flow-editor/src/**/*.stories.@(ts|tsx)",
      "../../apps/kapi-desktop/frontend/src/**/*.stories.@(ts|tsx)",
    ],
    // Scope the @neokapi/react transform to kapi-desktop's own source.
    // packages/ui hits an upstream transform bug for `{cond && <JSX/>}`
    // expressions; tracked in neokapi/neokapi-react.
    i18n: {
      include: [resolve(here, "../../apps/kapi-desktop/frontend/src")],
    },
  },
  import.meta,
);

config.staticDirs = ["../../apps/kapi-desktop/frontend/public"];

export default config;
