import { defineConfig } from "vite-plus";
import type { PluginOption } from "vite";
import neokapi from "@neokapi/kapi-react/vite";
import kapiReactConfig from "./kapi-react.config.json" with { type: "json" };

export default defineConfig({
  // Run the kapi-react transform in tests too, so components are exercised the
  // same way the shells build them (the shells import this same config file —
  // one componentMap keeps extract-CLI and build-time hashes identical).
  // Bounded to vite's PluginOption per apps/kapi-desktop/frontend/vite.config.ts.
  plugins: [
    neokapi({ mode: "runtime", componentMap: kapiReactConfig.componentMap }) as PluginOption,
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
