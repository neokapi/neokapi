import { dirname, resolve } from "path";
import { fileURLToPath } from "url";
import { defineConfig } from "vite-plus";

const __dirname = dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  resolve: {
    alias: {
      "@neokapi/ui-primitives": resolve(__dirname, "../../../packages/ui/src"),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/__tests__/setup.ts"],
    exclude: ["dist/**", "storybook-static/**", "node_modules/**"],
  },
  lint: {
    ignorePatterns: ["dist/**", "storybook-static/**"],
    options: {
      typeAware: false,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**", "storybook-static/**"],
  },
});
