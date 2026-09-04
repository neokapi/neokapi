/**
 * neokapi-i18n extract — walk every matched JSX/TSX file and emit
 * translatable content in one of two shapes:
 *
 *   1. Default: per-file .kbf.json under --out (default `./i18n/`),
 *      mirroring the source tree — so the default `src/**` glob lands
 *      catalogs under `i18n/src/`. Source living in `i18n/src/` (rather
 *      than flat under `i18n/`) leaves kapi free to write per-locale
 *      targets under `i18n/{lang}/` without the source glob re-ingesting
 *      them. Human-readable, git-diffable, self-contained per source.
 *      The tree stays a mirror: a catalog this run would have written
 *      for the document it records, and did not, is removed (see
 *      pruneStaleCatalogs). A catalog that is rewritten keeps the
 *      targets of every block whose content hash is unchanged (see
 *      carryTargets), so re-running the extract over an in-place
 *      translated tree adds and drops source blocks without discarding
 *      the translations that still apply.
 *   2. --stream: NDJSON block records on stdout, one per block,
 *      for piping into any kapi-aware consumer (e.g. kapi's exec
 *      format reader). No file output.
 *
 * Both shapes carry the same Block data; --stream is just the wire
 * form for a pipe. Warnings (unknown components) are always routed
 * to stderr.
 */

import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  rmdirSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { glob } from "node:fs/promises";

import type { Block, Document, File } from "@neokapi/kapi-format";
import { Ext, isKbfPath, marshalFile } from "@neokapi/kapi-format";

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

  const documents = extractAllDocuments(files, config, {
    strict: opts.strict,
    sourceRoot: opts.sourceRoot,
  });

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
  }

  // Per-file KBF under --out. One file per source document — the
  // human-readable, git-diffable on-disk shape. Kapi reads these
  // directly for translation / compile / check flows.
  const written = new Set<string>();
  const carried: CarrySummary = { kept: 0, dropped: 0, locales: new Set() };
  if (documents.length > 0) mkdirSync(opts.outDir, { recursive: true });
  for (const doc of documents) {
    const path = join(opts.outDir, kbfFilename(doc));
    carryTargets(doc, readTranslated(path), carried);
    const kbf = buildKBF(doc, opts);
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, marshalFile(kbf));
    written.add(resolve(path));
  }
  // Files were scanned, so the tree is the mirror of what they hold: whatever
  // this run did not write, and would have for the document it records, is
  // the catalog of a source that is gone.
  const stale = pruneStaleCatalogs(opts.outDir, written);
  if (documents.length > 0) {
    const blockCount = documents.reduce((n, d) => n + d.blocks.length, 0);
    console.log(`Extracted ${blockCount} blocks from ${documents.length} files → ${opts.outDir}/`);
  }
  if (carried.kept > 0) {
    const locales = [...carried.locales].sort().join(", ");
    console.log(`Kept the targets of ${carried.kept} unchanged block(s) (${locales})`);
  }
  if (carried.dropped > 0) {
    console.log(
      `Dropped the targets of ${carried.dropped} block(s) whose source changed or is gone`,
    );
  }
  if (stale.length > 0) {
    console.log(`Removed ${stale.length} stale catalog(s) whose source is no longer scanned:`);
    for (const path of stale) console.log(`  ${path}`);
  }
}

interface CarrySummary {
  /** Blocks that received the targets of their previous extraction. */
  kept: number;
  /** Translated blocks of the previous extraction that no current block matches. */
  dropped: number;
  /** Every locale carried across, for the report. */
  locales: Set<string>;
}

/** What a previous extraction recorded for one block: its targets and their provenance. */
type Translated = Pick<Block, "targets" | "targetOrigins">;

/**
 * The translated blocks of the catalog already at `path`, keyed by content
 * hash. Empty when there is no catalog there, when it does not parse as a
 * bundle, or when nothing in it carries a target.
 *
 * Keyed by hash and never by id: a block's id spells its line and position,
 * which move with every edit above it, while its hash is a function of its
 * source runs and its element alone. A target recorded under a hash is a
 * translation of exactly the content that hash names, wherever that content
 * now sits in the file. Two blocks in one file that share a hash share the
 * string, and the runtime dictionary keys on the hash, so the first recorded
 * target stands for both.
 */
function readTranslated(path: string): Map<string, Translated> {
  const out = new Map<string, Translated>();
  if (!existsSync(path)) return out;
  let file: File;
  try {
    file = JSON.parse(readFileSync(path, "utf-8")) as File;
  } catch {
    return out;
  }
  const documents = Array.isArray(file?.documents) ? file.documents : [];
  for (const doc of documents) {
    const blocks = Array.isArray(doc?.blocks) ? doc.blocks : [];
    for (const block of blocks) {
      if (typeof block?.hash !== "string" || out.has(block.hash)) continue;
      const targets = block.targets;
      if (!targets || typeof targets !== "object" || Object.keys(targets).length === 0) continue;
      out.set(block.hash, {
        targets,
        ...(block.targetOrigins && typeof block.targetOrigins === "object"
          ? { targetOrigins: block.targetOrigins }
          : {}),
      });
    }
  }
  return out;
}

/**
 * Give every block of `doc` the targets its previous extraction recorded for
 * the same content hash, and account for what was kept and what was left
 * behind.
 *
 * A block is written with a target only when its hash is unchanged: the same
 * source runs under the same element, so the translation still answers the
 * source beside it. A block whose text or element changed has a new hash,
 * matches nothing, and is written source-only to be translated again; a block
 * that was removed is simply absent, and its targets go with it. A target is
 * carried together with its provenance, so a bundle re-read by kapi still
 * says how each answer was produced.
 *
 * This is what makes the in-place layout survive a re-extract. The per-locale
 * layout, where kapi writes targets under `i18n/{lang}/`, is untouched by the
 * extract in the first place: those files sit outside the positions it writes.
 */
function carryTargets(doc: Document, previous: Map<string, Translated>, summary: CarrySummary) {
  if (previous.size === 0) return;
  const matched = new Set<string>();
  for (const block of doc.blocks) {
    const kept = previous.get(block.hash);
    if (!kept) continue;
    block.targets = kept.targets;
    if (kept.targetOrigins) block.targetOrigins = kept.targetOrigins;
    for (const locale of Object.keys(kept.targets ?? {})) summary.locales.add(locale);
    summary.kept += 1;
    matched.add(block.hash);
  }
  for (const hash of previous.keys()) {
    if (!matched.has(hash)) summary.dropped += 1;
  }
}

/**
 * Remove every catalog under `outDir` that this run did not write and would
 * have written for the document it records.
 *
 * The tree mirrors the sources scanned. A catalog for a component that was
 * deleted, renamed or dropped from `--src` has no place in the mirror, and left
 * behind it is translated and compiled like any other: the runtime dictionary
 * ships the strings of a component nobody can reach, and a checkout that has
 * run the extract for weeks regenerates different bytes from a clean one.
 *
 * Ownership is decided by position, never by content. A file is this
 * extractor's only if it sits exactly where `kbfFilename` places the document it
 * records; that is true of a catalog written by an earlier run under any
 * `--source-root`, and false of the per-locale targets kapi writes under
 * `i18n/{lang}/`, which record the same document path but sit elsewhere. A file
 * that does not parse as a bundle is nobody's to judge and is left alone. Only
 * a directory emptied by a removal is removed with it, and `outDir` itself never
 * is.
 */
function pruneStaleCatalogs(outDir: string, written: ReadonlySet<string>): string[] {
  if (!existsSync(outDir)) return [];
  const removed: string[] = [];

  // Returns whether `dir` is now empty and whether the walk removed anything
  // beneath it: a directory is deleted only when both hold.
  const walk = (dir: string): { empty: boolean; pruned: boolean } => {
    let empty = true;
    let pruned = false;
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) {
        const below = walk(path);
        if (below.empty && below.pruned) {
          rmdirSync(path);
          pruned = true;
        } else {
          empty = false;
        }
        continue;
      }
      if (
        entry.isFile() &&
        isKbfPath(path) &&
        !written.has(resolve(path)) &&
        isOwnCatalog(outDir, path)
      ) {
        unlinkSync(path);
        removed.push(path);
        pruned = true;
        continue;
      }
      empty = false;
    }
    return { empty, pruned };
  };

  walk(outDir);
  return removed.sort();
}

// A bundle is the extractor's own when one of its documents would be written
// exactly here.
function isOwnCatalog(outDir: string, path: string): boolean {
  let file: File;
  try {
    file = JSON.parse(readFileSync(path, "utf-8")) as File;
  } catch {
    return false;
  }
  const documents = Array.isArray(file?.documents) ? file.documents : [];
  const here = resolve(path);
  return documents.some(
    (doc) => typeof doc?.path === "string" && resolve(join(outDir, kbfFilename(doc))) === here,
  );
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
  // --source-root: the directory every recorded source path is relative TO.
  // Declared rather than derived; see resolveSourcePath. Empty means cwd.
  sourceRoot: string;
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
    sourceRoot: "",
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
      case "--source-root":
        if (value) parsed.sourceRoot = args[++i];
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

/**
 * The path a document is recorded under: the source file, relative to the root
 * this run was given.
 *
 * One path, and it carries two jobs — identity (it spells `doc.path`, `doc.id`
 * and every block id under it) and meaning (it is what a reviewer is shown).
 * That is why the root is DECLARED and not incidental. Left to the working
 * directory, a file scanned as `--src "../../apps/bowrain/frontend/src/**"`
 * records as `../../apps/bowrain/frontend/src/App.tsx`, which names a real file
 * only to someone who knows which directory the build ran in — and a surface
 * holding the catalog, an ocean away from the checkout, does not. Given
 * `--source-root ../../..` the same file records as
 * `bowrain/apps/bowrain/frontend/src/App.tsx`, which needs no such context.
 *
 * Declared, never derived. A root inferred from the common ancestor of whatever
 * `--src` globs matched would make every path — and so every block id — a
 * function of which files happened to exist: add a root reaching one level
 * further up and the whole collection re-keys. A root someone wrote down moves
 * when they move it.
 *
 * Without the flag this is the working directory, so a surface scanning only its
 * own `src/` records exactly what it always did.
 */
function resolveSourcePath(file: string, sourceRoot: string): string {
  return relative(sourceRoot ? resolve(sourceRoot) : process.cwd(), resolve(file));
}

function extractAllDocuments(
  files: readonly string[],
  config: ExtractConfig,
  { strict, sourceRoot }: { strict: boolean; sourceRoot: string } = {
    strict: false,
    sourceRoot: "",
  },
): Document[] {
  const out: Document[] = [];
  const warnings = createWarningCollector();
  for (const file of files) {
    const code = readFileSync(file, "utf-8");
    const filename = resolveSourcePath(file, sourceRoot);
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

function buildKBF(doc: Document, opts: ExtractArgs) {
  return {
    schemaVersion: "1.0" as const,
    kind: "kapi-bundle" as const,
    generator: { id: "@neokapi/i18n-react", version: readPackageVersion() },
    project: {
      id: opts.projectId,
      sourceLocale: opts.sourceLocale,
      ...(opts.targetLocales.length > 0 ? { targetLocales: opts.targetLocales } : {}),
    },
    documents: [doc],
  };
}

function kbfFilename(doc: Pick<Document, "path">): string {
  // Keep the source file's path shape inside --out so translators
  // scanning the directory see a 1:1 reflection of the source tree.
  //
  // A path that still climbs above the root — a `--src` reaching outside a
  // --source-root nobody widened to match — would escape --out and scatter
  // .kbf.json files into the library tree. Those segments are dropped rather
  // than honoured: a catalog written outside its own collection is worse than
  // one whose name lost a level, and declaring the root is what keeps the
  // situation from arising. doc.path keeps the path whole either way.
  const contained = doc.path.replace(/^(\.\.\/)+/, "");
  return contained.replace(/\.(tsx|jsx|ts|js)$/, "") + Ext;
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
neokapi-i18n extract — scan JSX/TSX files and emit translatable blocks.

Usage:
  neokapi-i18n extract [options]

By default, writes one .kbf.json file per source document under --out.
Pass --stream to emit NDJSON block records to stdout for piping.

Options:
  --src <glob>            Source files to scan (repeatable; default:
                          "src/**/*.{tsx,jsx}"). Pass multiple when your
                          app pulls translatable JSX from workspace
                          packages, e.g. --src "src/**/*.tsx" --src
                          "../../packages/ui/src/**/*.tsx"
  --ignore <glob>         Exclude pattern (repeatable). E.g. --ignore "src/stories/**"
  --out <dir>             Output directory for .kbf.json files (default: "i18n").
                          The tree mirrors the files scanned: a catalog left
                          there for a source this run no longer scans is
                          removed. A catalog that is rewritten keeps the
                          targets of every block whose content hash is
                          unchanged; a block whose text or element changed
                          is written without targets. Per-locale targets
                          kapi writes under a subdirectory of it are left
                          alone.
  --source-root <dir>     Directory every recorded source path is relative to
                          (default: the working directory). Declare it wherever
                          --src reaches outside the working directory, so a
                          catalog names its source the same way from anywhere —
                          it is the document's identity as well as what a
                          reviewer reads, so changing it re-keys the catalogs.
                          Catalogs mirror the source tree, so the default
                          "src/**" glob writes them under "i18n/src/" —
                          leaving "i18n/{lang}/" free for kapi's per-locale
                          targets.
  --stream                Emit NDJSON block records on stdout instead
                          of writing .kbf.json files. Reads NUL-separated
                          paths on stdin instead of expanding --src.
  --strict                Treat any recorded warning (e.g. unknown
                          component) as an error — exits non-zero.
  --config <path>         Config file with componentMap, rules, …
  --project <id>          Project id stamped into the catalog's project
                          field (default: "app")
  --source-locale <bcp>   Manifest source locale (default: "en")
  --target-locale <bcp>   Declared target locale (repeatable, informational)
`;
