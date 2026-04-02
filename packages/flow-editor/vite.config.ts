import path from "node:path";
import { defineConfig } from "vite-plus";

export default defineConfig({
  resolve: {
    alias: {
      "@neokapi/ui-primitives": path.resolve(__dirname, "../ui/src"),
    },
  },
  test: {
    environment: "node",
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
