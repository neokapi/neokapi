import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
export default defineConfig({
  plugins: [react(), tailwindcss()],
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
    ignorePatterns: ["dist/**", "bindings/**"],
    options: {
      typeAware: true,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**", "bindings/**"],
  },
});
