import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { keycloakify } from "keycloakify/vite-plugin/index.js";
import path from "path";

const __dirname = import.meta.dirname;

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    keycloakify({
      themeName: "bowrain",
      accountThemeImplementation: "none",
    }),
  ],
  resolve: {
    alias: {
      "@neokapi/ui": path.resolve(__dirname, "../../packages/ui/src"),
    },
  },
  lint: {
    ignorePatterns: ["dist/**", "dist_keycloak/**"],
    options: {
      typeAware: true,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**", "dist_keycloak/**"],
  },
});
