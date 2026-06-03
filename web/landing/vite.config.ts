import { defineConfig } from "vite-plus";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";

// Build freshness stamp ("<YYYY-MM-DD HH:MM> UTC · <short-sha>"), injected at
// build time so the deployed page shows when and from what commit it was built.
const gitSha = (() => {
  try {
    return execFileSync("git", ["rev-parse", "--short", "HEAD"], { stdio: ["ignore", "pipe", "ignore"] })
      .toString()
      .trim();
  } catch {
    return process.env.GITHUB_SHA?.slice(0, 9) ?? "dev";
  }
})();
const buildStamp = `${new Date().toISOString().slice(0, 16).replace("T", " ")} UTC · ${gitSha}`;

export default defineConfig({
  base: process.env.VITE_BASE ?? "/web/neokapi/",
  define: { __BUILD_STAMP__: JSON.stringify(buildStamp) },
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: "dist",
  },
  lint: {
    ignorePatterns: ["dist/**"],
  },
  fmt: {
    ignorePatterns: ["dist/**"],
  },
});
