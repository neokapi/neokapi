import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import type { Plugin } from "vite";
import neokapi from "@neokapi/react/vite";

// Scope @neokapi/react to this app's own source. Workspace deps under
// packages/ui use JSXExpressionContainer wrappers around inline JSX
// (e.g. `{cond && <Icon/>}`), which the runtime-mode transform currently
// coerces to "[object Object]" via template-literal interpolation.
// Tracked upstream — see neokapi/neokapi-react.
const here = resolve(fileURLToPath(import.meta.url), "..");
const appSrc = resolve(here, "src");
const raw = neokapi({ mode: "runtime" }) as Plugin;
const rawTransform = raw.transform as
  | ((this: unknown, code: string, id: string) => unknown)
  | undefined;
const scopedNeokapi: Plugin = {
  ...raw,
  transform(code, id) {
    if (!id.startsWith(appSrc)) return null;
    return rawTransform ? rawTransform.call(this, code, id) : null;
  },
};

export default defineConfig({
  plugins: [scopedNeokapi, react(), tailwindcss()],
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
    ignorePatterns: [
      "dist/**",
      "bindings/**",
      "storybook-static/**",
      "public/translations/**",
      "i18n/**",
    ],
    options: {
      typeAware: true,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: [
      "dist/**",
      "bindings/**",
      "storybook-static/**",
      "public/translations/**",
      "i18n/**",
    ],
  },
});
