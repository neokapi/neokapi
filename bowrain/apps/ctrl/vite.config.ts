import { defineConfig } from "vite-plus";
import type { PluginOption } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import neokapi from "@neokapi/i18n-react/vite";
import neokapiI18nConfig from "../../packages/app/neokapi-i18n.config.json" with { type: "json" };

export default defineConfig({
  // neokapi() is bounded to vite's own PluginOption — see apps/kapi-desktop/
  // frontend/vite.config.ts. componentMap is shared with the extract CLI
  // (bowrain/packages/app/neokapi-i18n.config.json) so hashes match the catalog.
  plugins: [
    neokapi({ mode: "runtime", componentMap: neokapiI18nConfig.componentMap }) as PluginOption,
    react(),
    tailwindcss(),
  ],
  server: {
    open: "https://ctrl.bowrain.mymac",
    port: 3100,
    proxy: {
      "/api": "http://localhost:8080",
    },
    allowedHosts: true,
  },
  build: {
    outDir: "dist",
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            { name: "vendor-router", test: /@tanstack[\\/]react-router/ },
            { name: "vendor-query", test: /@tanstack[\\/]react-query/ },
          ],
        },
      },
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
  },
  lint: {
    ignorePatterns: ["dist/**", "public/translations/**", "i18n/**", "i18n-qps/**", "i18n-nb/**"],
    options: {
      typeAware: true,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**", "public/translations/**", "i18n/**", "i18n-qps/**", "i18n-nb/**"],
  },
});
