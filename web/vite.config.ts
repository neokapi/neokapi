import { defineConfig } from "vite-plus";

// Docusaurus preserves JSX for its own compiler. Component tests need Vite to
// transform it before import analysis, independently of the site build.
export default defineConfig({
  oxc: { jsx: { runtime: "automatic" } },
  test: {
    environment: "node",
    exclude: ["build/**", ".docusaurus/**", "node_modules/**"],
  },
});
