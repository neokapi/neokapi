import { defineConfig } from "vite-plus";
import type { PluginOption } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import neokapi from "@neokapi/kapi-react/vite";
import kapiReactConfig from "../../packages/app/kapi-react.config.json" with { type: "json" };

export default defineConfig({
  // neokapi() is bounded to vite's own PluginOption to stop a type-instantiation
  // overflow against vite-plus's UserConfig — see apps/kapi-desktop/frontend/
  // vite.config.ts for the full rationale. The componentMap is the SAME file the
  // extract CLI reads (bowrain/packages/app/kapi-react.config.json) so the
  // build-time transform and the extracted catalogs hash identically.
  plugins: [
    neokapi({ mode: "runtime", componentMap: kapiReactConfig.componentMap }) as PluginOption,
    react(),
    tailwindcss(),
  ],
  server: {
    open: "https://bowrain.mymac",
    port: 5173,
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
    setupFiles: ["./src/__tests__/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
  lint: {
    ignorePatterns: ["dist/**", "public/translations/**"],
    options: {
      typeAware: true,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**", "public/translations/**"],
  },
});
