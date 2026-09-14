#!/usr/bin/env node
// Writes the comment spans the comment conformance tests hold the TypeScript,
// TSX and JavaScript providers to.
//
// The spans come from @babel/parser, which shares no code with the grammars the
// plugin uses, so the Go tests compare two independent readings of the same
// bytes and need no node of their own. Each fixture under corpus/<language>/
// has a golden beside it, <fixture>.babel, holding the parser's version, the
// file the fixture was copied from, the fixture's sha256, and one line per
// comment: start and end byte offsets, then the lengths of its opening and
// closing markers. A shebang is listed as a comment with a two-byte marker.
//
// Usage, from the repository root:
//
//   node plugins/sourcecode/internal/comments/testdata/babel-goldens.mjs
//       regenerates every golden
//   node plugins/sourcecode/internal/comments/testdata/babel-goldens.mjs --add <language> <file>...
//       copies repository files into the corpus and writes their goldens
//
// BABEL_PARSER names the parser's package directory when the repository has no
// node_modules to resolve it from, such as in a git worktree.

import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const corpus = path.join(here, "corpus");
const pluginsFor = {
  typescript: ["typescript", "decorators-legacy"],
  tsx: ["typescript", "jsx", "decorators-legacy"],
  javascript: ["jsx"],
};

function repoRoot() {
  return execFileSync("git", ["rev-parse", "--show-toplevel"], { cwd: here, encoding: "utf8" }).trim();
}

// loadParser resolves @babel/parser from BABEL_PARSER, the repository's
// node_modules, or the pnpm store of the main checkout a worktree belongs to.
function loadParser() {
  const root = repoRoot();
  const candidates = [];
  if (process.env.BABEL_PARSER) candidates.push(process.env.BABEL_PARSER);
  const common = execFileSync("git", ["rev-parse", "--path-format=absolute", "--git-common-dir"], {
    cwd: here,
    encoding: "utf8",
  }).trim();
  for (const base of [root, path.dirname(common)]) {
    const store = path.join(base, "node_modules", ".pnpm");
    if (!existsSync(store)) continue;
    const dirs = readdirSync(store)
      .filter((d) => d.startsWith("@babel+parser@"))
      .sort()
      .reverse();
    for (const d of dirs) candidates.push(path.join(store, d, "node_modules", "@babel", "parser"));
  }
  for (const dir of candidates) {
    if (!existsSync(path.join(dir, "package.json"))) continue;
    const require = createRequire(path.join(dir, "package.json"));
    const pkg = JSON.parse(readFileSync(path.join(dir, "package.json"), "utf8"));
    return { parser: require(dir), version: pkg.version };
  }
  throw new Error("@babel/parser not found: run `vp install` at the repository root, or set BABEL_PARSER");
}

// byteOffsets maps each UTF-16 index of text, which is what the parser reports,
// to the byte offset of the same position in its UTF-8 encoding.
function byteOffsets(text) {
  const offsets = new Array(text.length + 1);
  let bytes = 0;
  for (let i = 0; i < text.length; i++) {
    offsets[i] = bytes;
    const code = text.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff && i + 1 < text.length) {
      offsets[i + 1] = bytes;
      bytes += 4;
      i++;
    } else if (code < 0x80) {
      bytes += 1;
    } else if (code < 0x800) {
      bytes += 2;
    } else {
      bytes += 3;
    }
  }
  offsets[text.length] = bytes;
  return offsets;
}

function spans(parser, language, data) {
  const text = data.toString("utf8");
  if (!Buffer.from(text, "utf8").equals(data)) throw new Error("not valid UTF-8");
  const ast = parser.parse(text, {
    sourceType: "unambiguous",
    allowReturnOutsideFunction: true,
    allowAwaitOutsideFunction: true,
    allowImportExportEverywhere: true,
    allowUndeclaredExports: true,
    errorRecovery: false,
    plugins: pluginsFor[language],
  });
  const at = byteOffsets(text);
  const units = [];
  const interpreter = ast.program.interpreter;
  if (interpreter) units.push([at[interpreter.start], at[interpreter.end], 2, 0]);
  for (const c of ast.comments) {
    units.push([at[c.start], at[c.end], 2, c.type === "CommentBlock" ? 2 : 0]);
  }
  units.sort((a, b) => a[0] - b[0]);
  return units;
}

function writeGolden({ parser, version }, language, fixture, source) {
  const data = readFileSync(fixture);
  const lines = [
    `# comment spans from @babel/parser ${version}`,
    `# source ${source}`,
    `# sha256 ${createHash("sha256").update(data).digest("hex")}`,
  ];
  for (const u of spans(parser, language, data)) lines.push(u.join(" "));
  writeFileSync(`${fixture}.babel`, lines.join("\n") + "\n");
}

function sourceOf(golden) {
  if (!existsSync(golden)) return "authored";
  const line = readFileSync(golden, "utf8")
    .split("\n")
    .find((l) => l.startsWith("# source "));
  return line ? line.slice("# source ".length) : "authored";
}

function main() {
  const babel = loadParser();
  const args = process.argv.slice(2);
  if (args[0] === "--add") {
    const [, language, ...files] = args;
    if (!pluginsFor[language] || files.length === 0) {
      throw new Error("usage: --add <typescript|tsx|javascript> <file>...");
    }
    const root = repoRoot();
    mkdirSync(path.join(corpus, language), { recursive: true });
    for (const file of files) {
      const rel = path.relative(root, path.resolve(file)).split(path.sep).join("/");
      const name = rel.split("/").slice(-2).join("__") + ".txt";
      const fixture = path.join(corpus, language, name);
      writeFileSync(fixture, readFileSync(path.resolve(file)));
      writeGolden(babel, language, fixture, rel);
      console.log(`added ${path.relative(root, fixture)}`);
    }
    return;
  }
  for (const language of Object.keys(pluginsFor)) {
    const dir = path.join(corpus, language);
    if (!existsSync(dir)) continue;
    for (const name of readdirSync(dir).sort()) {
      if (!name.endsWith(".txt")) continue;
      const fixture = path.join(dir, name);
      writeGolden(babel, language, fixture, sourceOf(`${fixture}.babel`));
    }
  }
}

main();
