import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
export default defineConfig({
  plugins: [react(), tailwindcss()],
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
    ignorePatterns: ["dist/**", "bindings/**"],
    options: {
      typeAware: true,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**", "bindings/**"],
  },
});
