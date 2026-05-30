import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";

// The Office task pane is a standalone React SPA loaded by Office in an iframe.
// It calls the Bowrain add-in REST API (/api/v1/addin/*); in dev that is proxied
// to a locally running bowrain-server.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 3300,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
  build: { outDir: "dist" },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/__tests__/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
  lint: {
    ignorePatterns: ["dist/**"],
    options: { typeAware: true, typeCheck: false },
  },
  fmt: {
    singleQuote: false,
    ignorePatterns: ["dist/**"],
  },
});
