import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

const __dirname = import.meta.dirname;

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@neokapi/ui-primitives": path.resolve(__dirname, "../../../../packages/ui/src"),
      "@neokapi/flow-editor": path.resolve(__dirname, "../../../../packages/flow-editor/src"),
      // Force all packages to use the same React instance (prevents
      // duplicate React from packages/flow-editor/node_modules/).
      react: path.resolve(__dirname, "node_modules/react"),
      "react-dom": path.resolve(__dirname, "node_modules/react-dom"),
    },
    dedupe: ["react", "react-dom", "@xyflow/react", "@xyflow/system"],
  },
  build: {
    outDir: "dist",
  },
  server: {
    port: 5174,
    strictPort: true,
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/__tests__/setup.ts"],
    exclude: ["dist/**", "storybook-static/**", "node_modules/**"],
  },
  lint: {
    ignorePatterns: ["dist/**", "bindings/**", "storybook-static/**"],
    options: {
      typeAware: true,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**", "bindings/**", "storybook-static/**"],
  },
});
