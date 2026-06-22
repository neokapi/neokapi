import { defineConfig } from "vite-plus";
import type { PluginOption } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import neokapi from "@neokapi/kapi-react/vite";

export default defineConfig({
  // neokapi() is an unplugin `.vite` adapter; its deeply-inferred return type
  // overflows TypeScript's instantiation-depth limit when compared against
  // vite-plus's (rolldown-based) UserConfig. Bounding it to vite's own
  // PluginOption keeps the plugin fully type-safe while stopping the recursion.
  plugins: [
    // componentMap stabilises i18n hashes for app-local wrapper components
    // that render translatable text — without it the extractor warns and the
    // hash could shift once a mapping is added. Map each to its rendered tag.
    neokapi({ mode: "runtime", componentMap: { TabButton: "button" } }) as PluginOption,
    react(),
    tailwindcss(),
  ],
  build: {
    outDir: "dist",
  },
  server: {
    // Bind IPv4 loopback explicitly. "localhost" resolves to IPv6 (::1) on
    // macOS, but the Wails v3 dev asset proxy dials IPv4 (tcp4 127.0.0.1),
    // so without this the proxy gets "connection refused" and the dev window
    // stays blank.
    host: "127.0.0.1",
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
