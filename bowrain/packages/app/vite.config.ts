import { defineConfig } from "vite-plus";
import type { PluginOption } from "vite";
import neokapi from "@neokapi/i18n-react/vite";
import neokapiI18nConfig from "./neokapi-i18n.config.json" with { type: "json" };

export default defineConfig({
  // Run the neokapi-i18n transform in tests too, so components are exercised the
  // same way the shells build them (the shells import this same config file —
  // one componentMap keeps extract-CLI and build-time hashes identical).
  // Bounded to vite's PluginOption per apps/kapi-desktop/frontend/vite.config.ts.
  plugins: [
    neokapi({ mode: "runtime", componentMap: neokapiI18nConfig.componentMap }) as PluginOption,
  ],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test-setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    exclude: ["dist/**", "node_modules/**"],
  },
  lint: {
    ignorePatterns: ["dist/**", "i18n/**", "i18n-qps/**", "i18n-nb/**"],
    options: {
      typeAware: true,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**", "i18n/**", "i18n-qps/**", "i18n-nb/**"],
  },
});
