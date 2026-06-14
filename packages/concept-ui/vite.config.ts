import { defineConfig } from "vite-plus";

export default defineConfig({
  test: {
    // Pure-logic tests run in node; component tests opt into jsdom per-file
    // with a `// @vitest-environment jsdom` pragma.
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
