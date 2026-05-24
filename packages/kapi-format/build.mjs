#!/usr/bin/env node
import { build } from "esbuild";
import { rmSync, readFileSync, writeFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { execFileSync } from "node:child_process";

rmSync("dist", { recursive: true, force: true });

// Every source file is an entry point: bundle:false transpiles each in place so
// the published package keeps the same module graph (the subpath exports —
// ./block, ./klf, … — resolve to their own dist files).
const entryPoints = [
  "src/index.ts",
  "src/block.ts",
  "src/vocabulary.ts",
  "src/preview.ts",
  "src/annotation.ts",
  "src/klf.ts",
  "src/target-plural.ts",
  "src/runs.ts",
  "src/runs-validate.ts",
  "src/icu-parse.ts",
];

// Rewrites ./foo.ts → ./foo.js in relative imports so the emitted JS resolves at
// runtime. esbuild with bundle:false preserves import specifiers as-is, so we
// rewrite them in a transform step.
const rewriteTsExtensions = {
  name: "rewrite-ts-extensions",
  setup(b) {
    b.onLoad({ filter: /\.tsx?$/ }, (args) => {
      const raw = readFileSync(args.path, "utf8");
      const contents = raw
        .replace(/from\s+(['"])(\.\.?\/[^'"]+?)\.tsx?\1/g, "from $1$2.js$1")
        .replace(/import\(\s*(['"])(\.\.?\/[^'"]+?)\.tsx?\1\s*\)/g, "import($1$2.js$1)");
      return {
        contents,
        loader: args.path.endsWith(".tsx") ? "tsx" : "ts",
      };
    });
  },
};

await build({
  entryPoints,
  outdir: "dist",
  outbase: "src",
  bundle: false,
  format: "esm",
  platform: "neutral",
  target: "node22",
  sourcemap: true,
  plugins: [rewriteTsExtensions],
});

execFileSync("npx", ["tsc", "-p", "tsconfig.build.json"], { stdio: "inherit" });

// tsc's rewriteRelativeImportExtensions rewrites .ts → .js in JS output but not
// consistently in .d.ts output (type-only imports and dynamic import() type
// queries retain .ts). Fix up .d.ts files here.
function walk(dir) {
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) walk(p);
    else if (p.endsWith(".d.ts")) {
      const raw = readFileSync(p, "utf8");
      const fixed = raw
        .replace(/from\s+(['"])(\.\.?\/[^'"]+?)\.tsx?\1/g, "from $1$2.js$1")
        .replace(/import\((['"])(\.\.?\/[^'"]+?)\.tsx?\1\)/g, "import($1$2.js$1)");
      if (fixed !== raw) writeFileSync(p, fixed);
    }
  }
}
walk("dist");

console.log("Built dist/");
