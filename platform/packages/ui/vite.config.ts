import { dirname, resolve } from "path";
import { fileURLToPath } from "url";
import { defineConfig } from "vite-plus";

const __dirname = dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  resolve: {
    alias: {
      "@neokapi/ui-primitives": resolve(__dirname, "../../../packages/ui/src"),
      // Force all packages to resolve the same React instance — prevents
      // dual-instance errors when packages/ui/node_modules has its own copy.
      react: resolve(__dirname, "../../node_modules/react"),
      "react-dom": resolve(__dirname, "../../node_modules/react-dom"),
      "react/jsx-runtime": resolve(__dirname, "../../node_modules/react/jsx-runtime"),
      "react/jsx-dev-runtime": resolve(
        __dirname,
        "../../node_modules/react/jsx-dev-runtime",
      ),
    },
    dedupe: ["react", "react-dom"],
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/__tests__/setup.ts"],
    exclude: ["dist/**", "storybook-static/**", "node_modules/**"],
    server: {
      deps: {
        // Inline packages that import react from packages/ui/node_modules
        // so the resolve aliases above take effect.
        inline: [
          /@neokapi\/ui-primitives/,
          /radix-ui/,
          /@radix-ui/,
          /@base-ui/,
          /react-remove-scroll/,
          /recharts/,
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
