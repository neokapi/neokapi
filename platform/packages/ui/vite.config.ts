import { defineConfig } from "vite-plus";

export default defineConfig({
  test: {
    environment: "jsdom",
    setupFiles: ["./src/__tests__/setup.ts"],
    exclude: ["dist/**", "storybook-static/**", "node_modules/**"],
  },
  lint: {
    ignorePatterns: ["dist/**", "storybook-static/**"],
    options: {
      typeAware: true,
      typeCheck: true,
    },
  },
  fmt: {
    singleQuote: false,
  },
});
