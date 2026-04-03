import { dirname, resolve } from "path";
import { fileURLToPath } from "url";
import { defineConfig } from "vite-plus";

const __dirname = dirname(fileURLToPath(import.meta.url));

// Shared packages/ui lives outside the platform workspace. When tests import
// components from @neokapi/ui-primitives, their transitive deps (radix-ui,
// base-ui, recharts, etc.) resolve react from packages/ui/node_modules which
// is a different instance than platform's react. We fix this by:
// 1. Aliasing @neokapi/ui-primitives to the source directory
// 2. Aliasing react/react-dom to the platform's copy
// 3. Inlining packages that bundle their own react references

export default defineConfig({
  resolve: {
    alias: {
      "@neokapi/ui-primitives": resolve(__dirname, "../../../packages/ui/src"),
      react: resolve(__dirname, "../../node_modules/react"),
      "react-dom": resolve(__dirname, "../../node_modules/react-dom"),
      "react/jsx-runtime": resolve(__dirname, "../../node_modules/react/jsx-runtime"),
      "react/jsx-dev-runtime": resolve(__dirname, "../../node_modules/react/jsx-dev-runtime"),
    },
    dedupe: ["react", "react-dom"],
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/__tests__/setup.ts"],
    exclude: ["dist/**", "storybook-static/**", "node_modules/**"],
    server: {
      deps: {
        inline: [
          /radix-ui/,
          /@radix-ui/,
          /@base-ui/,
          /react-remove-scroll/,
          /recharts/,
          /@neokapi\/ui-primitives/,
        ],
      },
    },
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
