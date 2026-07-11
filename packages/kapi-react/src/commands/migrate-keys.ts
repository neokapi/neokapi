/**
 * kapi-react migrate-keys — one-shot migration from the v1 (pre-2.0)
 * key scheme to v2.
 *
 * v1 keys were 32-bit Jenkins hashes over a descriptor that included
 * the full JSX ancestor path; v2 keys are 64-bit FNV-1a hashes over
 * the element's own tag only. This command re-extracts every source
 * file computing BOTH hashes per block, builds the old → new mapping,
 * and rewrites:
 *
 *   --dicts <dir>   every {locale}.json compiled dictionary in place
 *   --klf <dir>     every .klf file's block hashes in place
 *                   (targets, notes, and all other block data are
 *                   preserved verbatim)
 *
 * Keys the mapping doesn't cover (removed blocks, strings whose text
 * changed since the dict was compiled) are kept and reported as
 * orphans — pass --prune to drop them. Recover orphaned translations
 * via kapi's TM (`kapi recycle`) after re-extracting.
 */

import { readFileSync, writeFileSync, readdirSync, statSync } from "node:fs";
import { glob } from "node:fs/promises";
import { join, relative } from "node:path";

import { extractDocument } from "../extract/index.ts";
import type { PluginOptions } from "../types.ts";

type MigrateConfig = Pick<PluginOptions, "componentMap" | "rules">;

export async function runMigrateKeys(args: string[]): Promise<void> {
  const opts = parseArgs(args);
  if (opts.help) {
    console.log(usage);
    return;
  }
  if (!opts.dictsDir && !opts.klfDir) {
    console.error("migrate-keys: pass --dicts <dir> and/or --klf <dir> to rewrite something.");
    console.log(usage);
    process.exitCode = 1;
    return;
  }

  const config = loadConfig(opts.configPath);
  const srcGlobs = opts.srcGlobs.length > 0 ? opts.srcGlobs : ["src/**/*.{tsx,jsx}"];

  // ── Build the old → new mapping from the current sources ──
  const mapping = new Map<string, string>();
  const conflicts: string[] = [];
  let files = 0;
  const globOptions = { exclude: [...opts.ignoreGlobs] };
  for (const pattern of srcGlobs) {
    for await (const file of glob(pattern, globOptions)) {
      files++;
      const code = readFileSync(file, "utf-8");
      extractDocument(code, {
        filename: relative(process.cwd(), file),
        ...config,
        onKeyPair: ({ legacyHash, hash }) => {
          if (!legacyHash) return;
          const existing = mapping.get(legacyHash);
          if (existing && existing !== hash) {
            conflicts.push(`${legacyHash} → ${existing} | ${hash}`);
            return;
          }
          mapping.set(legacyHash, hash);
        },
      });
    }
  }
  console.log(`Scanned ${files} files → ${mapping.size} key pairs.`);
  if (conflicts.length > 0) {
    console.warn(
      `WARNING: ${conflicts.length} v1 keys map to more than one v2 key (v1 hash ` +
        `collisions). First mapping wins; review:\n  ${conflicts.slice(0, 10).join("\n  ")}`,
    );
  }
  if (mapping.size === 0) {
    console.warn("No key pairs found — nothing to migrate.");
    return;
  }

  // ── Rewrite compiled dictionaries ──
  if (opts.dictsDir) {
    for (const entry of readdirSync(opts.dictsDir)) {
      if (!entry.endsWith(".json") || entry === "translations-manifest.json") continue;
      const path = join(opts.dictsDir, entry);
      const dict = JSON.parse(readFileSync(path, "utf-8")) as Record<string, string>;
      const out: Record<string, string> = {};
      let migrated = 0;
      const orphans: string[] = [];
      for (const [key, value] of Object.entries(dict)) {
        const next = mapping.get(key);
        if (next) {
          if (!(next in out)) out[next] = value;
          migrated++;
        } else if (opts.prune) {
          orphans.push(key);
        } else {
          out[key] = value;
          orphans.push(key);
        }
      }
      if (!opts.dryRun) writeFileSync(path, JSON.stringify(out, null, 2) + "\n");
      const verb = opts.dryRun ? "would migrate" : "migrated";
      console.log(
        `${entry}: ${verb} ${migrated}/${Object.keys(dict).length} keys` +
          (orphans.length > 0
            ? `, ${orphans.length} orphan${orphans.length === 1 ? "" : "s"}${opts.prune ? " pruned" : " kept"}`
            : ""),
      );
    }
  }

  // ── Rewrite .klf block hashes in place ──
  if (opts.klfDir) {
    let filesTouched = 0;
    let blocksMigrated = 0;
    let blockOrphans = 0;
    for (const path of walkKlf(opts.klfDir)) {
      const raw = JSON.parse(readFileSync(path, "utf-8")) as {
        documents?: Array<{ blocks?: Array<{ hash?: string }> }>;
      };
      let touched = false;
      for (const doc of raw.documents ?? []) {
        for (const block of doc.blocks ?? []) {
          if (!block.hash) continue;
          const next = mapping.get(block.hash);
          if (next) {
            block.hash = next;
            blocksMigrated++;
            touched = true;
          } else {
            blockOrphans++;
          }
        }
      }
      if (touched) {
        filesTouched++;
        if (!opts.dryRun) writeFileSync(path, JSON.stringify(raw, null, 2) + "\n");
      }
    }
    const verb = opts.dryRun ? "would migrate" : "migrated";
    console.log(
      `KLF: ${verb} ${blocksMigrated} block hashes across ${filesTouched} files` +
        (blockOrphans > 0 ? `, ${blockOrphans} blocks unmatched (re-extract to refresh)` : ""),
    );
  }

  if (opts.dryRun) console.log("(dry run — nothing written)");
}

function* walkKlf(dir: string): Generator<string> {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) yield* walkKlf(path);
    else if (entry.endsWith(".klf")) yield path;
  }
}

interface MigrateArgs {
  srcGlobs: string[];
  ignoreGlobs: string[];
  configPath: string | null;
  dictsDir: string | null;
  klfDir: string | null;
  prune: boolean;
  dryRun: boolean;
  help: boolean;
}

function parseArgs(args: string[]): MigrateArgs {
  const parsed: MigrateArgs = {
    srcGlobs: [],
    ignoreGlobs: [],
    configPath: null,
    dictsDir: null,
    klfDir: null,
    prune: false,
    dryRun: false,
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
      case "--config":
        if (value) parsed.configPath = args[++i];
        break;
      case "--dicts":
        if (value) parsed.dictsDir = args[++i];
        break;
      case "--klf":
        if (value) parsed.klfDir = args[++i];
        break;
      case "--prune":
        parsed.prune = true;
        break;
      case "--dry-run":
        parsed.dryRun = true;
        break;
      default:
        console.warn(`unknown flag: ${flag}`);
    }
  }
  return parsed;
}

function loadConfig(path: string | null): MigrateConfig {
  if (!path) return {};
  try {
    return JSON.parse(readFileSync(path, "utf-8")) as MigrateConfig;
  } catch (e) {
    console.error(`Failed to load config from ${path}:`, e);
    process.exit(1);
  }
}

const usage = `
kapi-react migrate-keys — migrate v1 (pre-2.0) keys to the v2 scheme.

Usage:
  kapi-react migrate-keys [--src <glob>]... [--dicts <dir>] [--klf <dir>] [options]

Re-extracts the current sources computing both the v1 and v2 hash per
block, then rewrites compiled dictionaries and/or .klf trees in place.
Run once when upgrading to @neokapi/kapi-react 2.x, commit the result.

Options:
  --src <glob>      Source files to scan (repeatable; default: "src/**/*.{tsx,jsx}")
  --ignore <glob>   Exclude pattern (repeatable) — mirror your extract flags
  --config <path>   Config file with componentMap, rules, …
  --dicts <dir>     Rewrite every {locale}.json dictionary in this directory
  --klf <dir>       Rewrite block hashes in every .klf under this directory
                    (targets and annotations are preserved)
  --prune           Drop dictionary keys the mapping doesn't cover
                    (default: keep + report as orphans)
  --dry-run         Report what would change without writing
`;
