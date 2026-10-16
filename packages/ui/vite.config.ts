import { defineConfig } from "vite-plus";

export default defineConfig({
  test: {
    environment: "node",
    exclude: ["dist/**", "storybook-static/**", "node_modules/**"],
  },
});
