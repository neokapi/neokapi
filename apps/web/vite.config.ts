import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@gokapi/ui": path.resolve(__dirname, "../../packages/ui/src"),
    },
  },
  build: {
    outDir: "dist",
  },
});
