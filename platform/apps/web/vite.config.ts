import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@neokapi/ui": path.resolve(__dirname, "../../packages/ui/ui/src"),
    },
  },
  server: {
    open: true,
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
    },
    allowedHosts: true,
  },
  build: {
    outDir: "dist",
    rollupOptions: {
      output: {
        manualChunks: {
          "vendor-router": ["@tanstack/react-router"],
          "vendor-query": ["@tanstack/react-query"],
        },
      },
    },
  },
});
