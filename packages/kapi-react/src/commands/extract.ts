/**
 * kapi-react extract — walk every matched JSX/TSX file and emit
 * translatable content in one of two shapes:
 *
 *   1. Default: per-file .klf JSON under --out (default `./i18n/`).
 *      Human-readable, git-diffable, self-contained per source.
 *   2. --stream: NDJSON block records on stdout, one per block,
 *      for piping into any kapi-aware consumer (e.g. kapi's exec
 *      format reader). No file output.
 *
 * Both shapes carry the same Block data; --stream is just the wire
 * form for a pipe. Warnings (unknown components) are always routed
 * to stderr.
 */

import { readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { glob } from "node:fs/promises";

import type { Document } from "@neokapi/kapi-format";
import { marshalFile } from "@neokapi/kapi-format";

import { createWarningCollector, extractDocument, formatWarning } from "../extract/index.ts";
import type { PluginOptions } from "../types.ts";

type ExtractConfig = Pick<PluginOptions, "componentMap" | "rules">;

export interface RunExtractIO {
  /** Source of NUL-separated paths for --stream mode. */
  stdin?: NodeJS.ReadableStream;
  /** Sink for NDJSON block records in --stream mode. */
  stdout?: NodeJS.WritableStream;
}

export async function runExtract(args: string[], io: RunExtractIO = {}): Promise<void> {
  const opts = parseArgs(args);
  if (opts.help) {
    console.log(usage);
    return;
  }

  const stdin = io.stdin ?? process.stdin;
  const stdout = io.stdout ?? process.stdout;

  const config = loadConfig(opts.configPath);

  // Default src when none specified. Can't set as a static default
  // in parseArgs because we need to know whether the user gave us
  // any --src themselves.
  const srcGlobs = opts.srcGlobs.length > 0 ? opts.srcGlobs : ["src/**/*.{tsx,jsx}"];

  // Stream mode accepts two shapes of file discovery, picked
  // automatically: when stdin is piped (e.g. kapi's exec format
  // sends NUL-separated paths), we consume it; otherwise we fall
  // back to the --src glob so a developer can just pipe our stdout
  // without also wiring up stdin.
  const files = opts.stream
    ? stdinHasInput(stdin)
      ? await readPathsFromStdin(stdin)
      : await expandGlobs(srcGlobs, opts.ignoreGlobs)
    : await expandGlobs(srcGlobs, opts.ignoreGlobs);
  files.sort();

  if (files.length === 0) {
    if (!opts.stream) console.warn(`No files found matching ${JSON.stringify(srcGlobs)}`);
    return;
  }

  if (!opts.stream) {
    console.log(`Scanning ${files.length} files...`);
  }

  const documents = extractAllDocuments(files, config, { strict: opts.strict });

  if (opts.stream) {
    // NDJSON block stream on stdout — consumed by kapi's exec
    // format reader or any other kapi-aware pipeline. No files
    // written here.
    for (const doc of documents) {
      for (const block of doc.blocks) {
        stdout.write(JSON.stringify({ type: "block", document: doc.path, block }) + "\n");
      }
    }
    return;
  }

  if (documents.length === 0) {
    console.warn("No translatable content found.");
    return;
  }

  // Per-file KLF under --out. One file per source document — the
  // human-readable, git-diffable on-disk shape. Kapi reads these
  // directly for translation / compile / QA flows.
  mkdirSync(opts.outDir, { recursive: true });
  for (const doc of documents) {
    const klf = buildKLF(doc, opts);
    const path = join(opts.outDir, klfFilename(doc));
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, marshalFile(klf));
  }
  const blockCount = documents.reduce((n, d) => n + d.blocks.length, 0);
  console.log(`Extracted ${blockCount} blocks from ${documents.length} files → ${opts.outDir}/`);
}

async function expandGlobs(
  patterns: readonly string[],
  ignore: readonly string[] = [],
): Promise<string[]> {
  // Node 22+'s `fs/promises.glob` accepts `{ exclude }` as a glob
  // pattern list. Pass our `--ignore` flags through untouched. The
  // options object is always non-undefined (even when empty) because
  // the glob type signature doesn't accept `undefined`.
  const options = { exclude: [...ignore] };
  const seen = new Set<string>();
  for (const pattern of patterns) {
    for await (const file of glob(pattern, options)) seen.add(file);
  }
  return [...seen];
}

// stdinHasInput returns true when stdin is piped / redirected — a
// signal from the shell that the caller has data for us. Falsey
// when stdin is inherited from a terminal (a user running the
// command interactively), in which case reading would block
// forever. Node sets `isTTY` on standard streams for us; falling
// back to the global process.stdin lets us probe without consuming.
function stdinHasInput(stdin: NodeJS.ReadableStream): boolean {
  // Test stream we were given first (unit tests pass a Readable
  // that has no isTTY flag — treat that as "has input" so tests
  // exercise the stdin path).
  const streamIsTTY = (stdin as { isTTY?: boolean }).isTTY;
  if (streamIsTTY === true) return false;
  if (streamIsTTY === false) return true;
  // No isTTY property (mock streams, Duplex wrappers): fall back
  // to the real process.stdin if we were passed it, otherwise
  // assume the caller piped something.
  if (stdin === process.stdin) return !process.stdin.isTTY;
  return true;
}

// readPathsFromStdin consumes NUL-separated paths from the given
// readable stream — the protocol kapi uses when invoking an
// exec-extractor. Filters empty segments and trims whitespace so a
// trailing newline or stray NUL doesn't produce a phantom path.
async function readPathsFromStdin(stdin: NodeJS.ReadableStream): Promise<string[]> {
  const chunks: Buffer[] = [];
  for await (const chunk of stdin) {
    if (Buffer.isBuffer(chunk)) chunks.push(chunk);
    else if (typeof chunk === "string") chunks.push(Buffer.from(chunk, "utf8"));
    else chunks.push(Buffer.from(chunk as unknown as ArrayBuffer));
  }
  const raw = Buffer.concat(chunks).toString("utf8");
  return raw
    .split("\0")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

// ─── Internals ────────────────────────────────────────────────────

interface ExtractArgs {
  srcGlobs: string[];
  ignoreGlobs: string[];
  outDir: string;
  configPath: string | null;
  projectId: string;
  sourceLocale: string;
  targetLocales: string[];
  // stream switches to NDJSON-on-stdout mode: reads NUL-separated
  // paths from stdin, never writes files. Used by `kapi extract`
  // (exec format) and other kapi-aware pipelines.
  stream: boolean;
  // --strict makes any recorded warning fail the run with a non-zero
  // exit. Intended for CI — see the lint plan in issue #381.
  strict: boolean;
  help: boolean;
}

function parseArgs(args: string[]): ExtractArgs {
  const parsed: ExtractArgs = {
    srcGlobs: [],
    ignoreGlobs: [],
    outDir: "i18n",
    configPath: null,
    projectId: "app",
    sourceLocale: "en",
    targetLocales: [],
    stream: false,
    strict: false,
    help: false,
  };

  for (let i = 0; i < args.length; i++) {
    const flag = args[i];
    const value = args[i + 1];
    switch (flag) {
      case "--help":
      case "-h":
        parsed.help = true;
        return parsed;
      case "--src":
        if (value) parsed.srcGlobs.push(args[++i]);
        break;
      case "--ignore":
        if (value) parsed.ignoreGlobs.push(args[++i]);
        break;
      case "--out":
        if (value) parsed.outDir = args[++i];
        break;
      case "--config":
        if (value) parsed.configPath = args[++i];
        break;
      case "--project":
        if (value) parsed.projectId = args[++i];
        break;
      case "--source-locale":
        if (value) parsed.sourceLocale = args[++i];
        break;
      case "--target-locale":
        if (value) parsed.targetLocales.push(args[++i]);
        break;
      case "--stream":
        parsed.stream = true;
        break;
      case "--strict":
        parsed.strict = true;
        break;
      default:
        console.warn(`unknown flag: ${flag}`);
    }
  }

  return parsed;
}

function loadConfig(path: string | null): ExtractConfig {
  if (!path) return {};
  try {
    return JSON.parse(readFileSync(path, "utf-8")) as ExtractConfig;
  } catch (e) {
    console.error(`Failed to load config from ${path}:`, e);
    process.exit(1);
  }
}

function extractAllDocuments(
  files: readonly string[],
  config: ExtractConfig,
  { strict }: { strict: boolean } = { strict: false },
): Document[] {
  const out: Document[] = [];
  const warnings = createWarningCollector();
  for (const file of files) {
    const code = readFileSync(file, "utf-8");
    const filename = relative(process.cwd(), file);
    const doc = extractDocument(code, { filename, warnings, ...config });
    if (doc) out.push(doc);
  }
  const list = warnings.list();
  for (const w of list) {
    console.warn(formatWarning(w));
  }
  if (strict && list.length > 0) {
    console.error(
      `[neokapi] --strict: ${list.length} warning${list.length === 1 ? "" : "s"} treated as errors. Exiting non-zero.`,
    );
    process.exit(1);
  }
  return out;
}

function buildKLF(doc: Document, opts: ExtractArgs) {
  return {
    schemaVersion: "1.0" as const,
    kind: "kapi-localization-format" as const,
    generator: { id: "@neokapi/kapi-react", version: readPackageVersion() },
    project: {
      id: opts.projectId,
      sourceLocale: opts.sourceLocale,
      ...(opts.targetLocales.length > 0 ? { targetLocales: opts.targetLocales } : {}),
    },
    documents: [doc],
  };
}

function klfFilename(doc: Document): string {
  // Keep the source file's path shape inside --out so translators
  // scanning the directory see a 1:1 reflection of the source tree.
  // Workspace sources outside the project root (e.g. --src
  // "../../packages/ui/src/**/*.tsx") carry leading "../" segments
  // that would escape --out and scatter .klf files into the library
  // tree; strip them so every output lands inside the --out
  // directory. doc.path itself keeps the original relative path.
  const contained = doc.path.replace(/^(\.\.\/)+/, "");
  return contained.replace(/\.(tsx|jsx|ts|js)$/, "") + ".klf";
}

function readPackageVersion(): string {
  try {
    const url = new URL("../../package.json", import.meta.url);
    const pkg = JSON.parse(readFileSync(url, "utf-8")) as { version?: string };
    return pkg.version ?? "0.0.0";
  } catch {
    return "0.0.0";
  }
}

const usage = `
kapi-react extract — scan JSX/TSX files and emit translatable blocks.

Usage:
  kapi-react extract [options]

By default, writes one .klf file per source document under --out.
Pass --stream to emit NDJSON block records to stdout for piping.

Options:
  --src <glob>            Source files to scan (repeatable; default:
                          "src/**/*.{tsx,jsx}"). Pass multiple when your
                          app pulls translatable JSX from workspace
                          packages, e.g. --src "src/**/*.tsx" --src
                          "../../packages/ui/src/**/*.tsx"
  --ignore <glob>         Exclude pattern (repeatable). E.g. --ignore "src/stories/**"
  --out <dir>             Output directory for .klf files (default: "i18n")
  --stream                Emit NDJSON block records on stdout instead
                          of writing .klf files. Reads NUL-separated
                          paths on stdin instead of expanding --src.
  --strict                Treat any recorded warning (e.g. unknown
                          component) as an error — exits non-zero.
  --config <path>         Config file with componentMap, rules, …
  --project <id>          Project id stamped into .klf.project (default: "app")
  --source-locale <bcp>   Manifest source locale (default: "en")
  --target-locale <bcp>   Declared target locale (repeatable, informational)
`;
