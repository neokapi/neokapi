import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "dist",
  },
  // `recharts` (bowrain's dashboard charts) is a CJS-heavy dep; under
  // rolldown-vite, CJS deps should be force-pre-bundled (optimizeDeps.include)
  // so rolldown converts CJS→ESM cleanly. (This cleared a `require_isUnsafe
  // Property` crash; note a separate rolldown/Oxc dev-transform "__name is not
  // defined" bug still blocks the bowrain-desktop RECORDER under `vp dev` —
  // present even on vite-plus 0.1.24 — tracked as an upstream issue.)
  optimizeDeps: {
    include: ["recharts"],
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
