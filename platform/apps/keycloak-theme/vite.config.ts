import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { keycloakify } from "keycloakify/vite-plugin";
import path from "path";

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
});
