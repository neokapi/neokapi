import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

const __dirname = import.meta.dirname;

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@neokapi/ui": path.resolve(__dirname, "../../packages/ui/src"),
    },
  },
  server: {
    open: "https://bowrain.mymac",
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
    },
    allowedHosts: true,
  },
  build: {
    outDir: "dist",
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            { name: "vendor-router", test: /@tanstack[\\/]react-router/ },
            { name: "vendor-query", test: /@tanstack[\\/]react-query/ },
          ],
        },
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/__tests__/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
  lint: {
    ignorePatterns: ["dist/**"],
    options: {
      typeAware: true,
      typeCheck: false,
    },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**"],
  },
});
