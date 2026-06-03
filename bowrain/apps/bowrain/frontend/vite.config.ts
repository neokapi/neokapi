import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "dist",
  },
  // `recharts` (bowrain's dashboard charts) is CJS-heavy; rolldown-vite's
  // optimizeDeps pre-bundle of it emitted esbuild interop helpers (`__name`,
  // `require_isUnsafeProperty`) without defining them, crashing real.html on
  // load under `vp dev` (recorder timeout). Exclude it from the pre-bundle so
  // it's served from its ESM build, and disable keepNames as a belt-and-braces.
  esbuild: { keepNames: false },
  optimizeDeps: {
    exclude: ["recharts"],
    esbuildOptions: { keepNames: false },
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
