import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "node",
    exclude: ["dist/**", "storybook-static/**", "node_modules/**"],
  },
});
