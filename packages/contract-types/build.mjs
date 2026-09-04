#!/usr/bin/env node
// Build @neokapi/contract-types for publishing: transpile each source file in
// place (no bundling, so the subpath exports, ./contract.gen, ./content.gen,
// ./review.gen and ./manual, resolve to their own dist files) and emit declarations with tsc.
// Mirrors packages/kapi-format/build.mjs; in-repo consumers use the TS source
// directly (see the package.json exports vs publishConfig split).
import { build } from "esbuild";
import { rmSync, readFileSync, writeFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { execFileSync } from "node:child_process";

rmSync("dist", { recursive: true, force: true });

const entryPoints = [
  "src/index.ts",
  "src/contract.gen.ts",
  "src/content.gen.ts",
  "src/review.gen.ts",
  "src/manual.ts",
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
        .replace(/import\(\s*(['"])(\.\.?\/[^'"]+?)\.tsx?\1\s*\)/g, "import($1$2.js$1)")
        .replace(/import\s+(['"])(\.\.?\/[^'"]+?)\.tsx?\1/g, "import $1$2.js$1");
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
  target: "es2022",
  sourcemap: true,
  plugins: [rewriteTsExtensions],
});

execFileSync("vpx", ["tsc", "-p", "tsconfig.build.json"], { stdio: "inherit" });

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
        .replace(/import\((['"])(\.\.?\/[^'"]+?)\.tsx?\1\)/g, "import($1$2.js$1)")
        .replace(/import\s+(['"])(\.\.?\/[^'"]+?)\.tsx?\1/g, "import $1$2.js$1");
      if (fixed !== raw) writeFileSync(p, fixed);
    }
  }
}
walk("dist");

console.log("Built dist/");
