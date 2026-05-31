#!/usr/bin/env node
import { build } from "esbuild";
import { rmSync, readFileSync, writeFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { execFileSync } from "node:child_process";

rmSync("dist", { recursive: true, force: true });

const entryPoints = [
  "src/cli.ts",
  "src/commands/extract.ts",
  "src/commands/compile.ts",
  "src/commands/split.ts",
  "src/extract/index.ts",
  "src/extract/walker.ts",
  "src/extract/runs.ts",
  "src/extract/translatable.ts",
  "src/extract/jsx-path.ts",
  "src/extract/ast.ts",
  "src/extract/plural.ts",
  "src/extract/messages.ts",
  "src/extract/warnings.ts",
  "src/types.ts",
  "src/plugin/index.ts",
  "src/plugin/vite.ts",
  "src/plugin/webpack.ts",
  "src/plugin/rollup.ts",
  "src/plugin/esbuild.ts",
  "src/plugin/transform.ts",
  "src/plugin/defaults.ts",
  "src/plugin/hash.ts",
  "src/plugin/manifests.ts",
  "src/plugin/chunk-manifest.ts",
  "src/runtime/index.ts",
  "src/runtime/icu.ts",
  "src/runtime/plural.tsx",
  "src/runtime/pseudo.ts",
  "src/storybook/index.ts",
];

// Rewrites ./foo.ts → ./foo.js in relative imports so the emitted JS
// resolves at runtime. esbuild with bundle:false preserves import
// specifiers as-is, so we rewrite them in a transform step.
const rewriteTsExtensions = {
  name: "rewrite-ts-extensions",
  setup(b) {
    b.onLoad({ filter: /\.tsx?$/ }, (args) => {
      const raw = readFileSync(args.path, "utf8");
      const contents = raw
        // static imports: from './foo.ts' → from './foo.js'
        .replace(/from\s+(['"])(\.\.?\/[^'"]+?)\.tsx?\1/g, "from $1$2.js$1")
        // dynamic imports: import('./foo.ts') → import('./foo.js')
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
  platform: "node",
  target: "node22",
  sourcemap: true,
  plugins: [rewriteTsExtensions],
});

execFileSync("vpx", ["tsc", "-p", "tsconfig.build.json"], { stdio: "inherit" });

// tsc's rewriteRelativeImportExtensions rewrites .ts → .js in JS output
// but not consistently in .d.ts output (type-only imports and dynamic
// import() type queries retain .ts). Fix up .d.ts files here.
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
