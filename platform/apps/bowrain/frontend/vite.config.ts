import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

const __dirname = import.meta.dirname;

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@neokapi/ui": path.resolve(__dirname, "../../../packages/ui/src"),
    },
    dedupe: ["react", "react-dom", "lexical", "@lexical/react"],
  },
  build: {
    outDir: "dist",
  },
  server: {
    port: 3000,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
  lint: {
    ignorePatterns: ["dist/**"],
    options: {
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**"],
  },
});
