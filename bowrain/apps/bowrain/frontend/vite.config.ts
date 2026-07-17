import { defineConfig } from "vite-plus";
import type { PluginOption } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import neokapi from "@neokapi/kapi-react/vite";
import kapiReactConfig from "../../../packages/app/kapi-react.config.json" with { type: "json" };

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
  build: {
    outDir: "dist",
  },
  // recharts is a CJS-heavy dep used by the LINKED workspace lib @neokapi/ui
  // (bowrain/packages/ui charts), not imported directly here. Vite's documented
  // form for a linked lib's nested CJS dep is `<linked-lib> > <nested-dep>`, so
  // it gets pre-bundled (CJS→ESM) with its helpers defined. A bare `recharts`
  // include is the wrong shape for a transitive-via-linked-lib dep and left the
  // rolldown helpers undefined ("__name is not defined") under `vp dev`.
  optimizeDeps: {
    include: ["@neokapi/ui > recharts"],
  },
  server: {
    host: "127.0.0.1",
    port: 3000,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/__tests__/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    exclude: ["dist/**", "node_modules/**", "e2e/**"],
  },
  lint: {
    ignorePatterns: ["dist/**", "bindings/**", "public/translations/**"],
    options: {
      typeAware: true,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**", "bindings/**", "public/translations/**"],
  },
});
