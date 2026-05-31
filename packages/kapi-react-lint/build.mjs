#!/usr/bin/env node
import { build } from "esbuild";
import { rmSync, readFileSync, writeFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { execFileSync } from "node:child_process";

rmSync("dist", { recursive: true, force: true });

const entryPoints = [
  "src/index.ts",
  "src/eslint.ts",
  "src/oxlint.ts",
  "src/rules/t-literal-first-arg.ts",
  "src/rules/t-no-concat.ts",
  "src/rules/no-concat-in-translatable-attr.ts",
  "src/rules/no-ternary-in-translatable-attr.ts",
  "src/rules/no-ternary-literals-in-jsx-child.ts",
  "src/rules/no-string-literal-jsx-expr.ts",
  "src/rules/prefer-t-for-label-props.ts",
  "src/rules/prefer-t-for-label-expr.ts",
  "src/shared/translatable-attrs.ts",
  "src/shared/translate-no.ts",
  "src/shared/t-import.ts",
  "src/configs/recommended.ts",
  "src/configs/recommended-strict.ts",
];

const rewriteTsExtensions = {
  name: "rewrite-ts-extensions",
  setup(b) {
    b.onLoad({ filter: /\.ts$/ }, (args) => {
      const raw = readFileSync(args.path, "utf8");
      const contents = raw
        .replace(/from\s+(['"])(\.\.?\/[^'"]+?)\.ts\1/g, "from $1$2.js$1")
        .replace(/import\(\s*(['"])(\.\.?\/[^'"]+?)\.ts\1\s*\)/g, "import($1$2.js$1)");
      return { contents, loader: "ts" };
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

function walk(dir) {
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) walk(p);
    else if (p.endsWith(".d.ts")) {
      const raw = readFileSync(p, "utf8");
      const fixed = raw
        .replace(/from\s+(['"])(\.\.?\/[^'"]+?)\.ts\1/g, "from $1$2.js$1")
        .replace(/import\((['"])(\.\.?\/[^'"]+?)\.ts\1\)/g, "import($1$2.js$1)");
      if (fixed !== raw) writeFileSync(p, fixed);
    }
  }
}
walk("dist");

console.log("Built dist/");
