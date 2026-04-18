/**
 * kapi-react compile — flatten translated blocks into runtime dicts.
 *
 * Accepts any of three input shapes:
 *
 *   1. A `.klz` archive (legacy + the canonical handoff format).
 *   2. A directory of `.klf` files (the KLF-default output of
 *      `kapi-react extract`).
 *   3. NDJSON block records on stdin (pass `-` as input) — for
 *      one-shot pipelines like
 *      `kapi extract-stream ... | kapi-react compile - --out ...`.
 *
 * Output: one `<locale>.json` dictionary per target locale, shape
 * `{ "<block.hash>": "<flattened target>" }`.
 */

import { readFileSync, writeFileSync, mkdirSync, readdirSync, statSync } from 'node:fs';
import { dirname, extname, join } from 'node:path';

import type { Block, File } from '@neokapi/kapi-format';
import { flattenRuns } from '@neokapi/kapi-format';
import { KlzReader } from '@neokapi/kapi-format/klz';

interface BlockRecord {
  block: Block;
}

export async function runCompile(args: string[]) {
  let input: string | null = null;
  const locales: string[] = [];
  let outDir = 'public/translations';

  for (let i = 0; i < args.length; i++) {
    const flag = args[i];
    const value = args[i + 1];
    if (flag === '--help' || flag === '-h') {
      console.log(usage);
      return;
    }
    if (flag === '--out' && value) outDir = args[++i];
    else if (flag === '--locale' && value) locales.push(args[++i]);
    else if (!flag.startsWith('--')) input = flag;
  }

  if (!input) {
    console.error('error: missing input (.klz, .klf directory, or - for stdin)\n');
    console.log(usage);
    process.exit(1);
  }

  const { blocks, declaredTargets } = await loadBlocks(input);

  // Infer the set of target locales when --locale wasn't passed.
  const targetLocales = new Set<string>(locales);
  if (targetLocales.size === 0) {
    for (const l of declaredTargets) targetLocales.add(l);
    for (const { block } of blocks) {
      for (const l of Object.keys(block.targets ?? {})) targetLocales.add(l);
    }
  }

  if (targetLocales.size === 0) {
    console.error('error: input has no target locales; pass --locale explicitly');
    process.exit(1);
  }

  mkdirSync(outDir, { recursive: true });

  let totalCompiled = 0;
  for (const locale of targetLocales) {
    const dict: Record<string, string> = {};
    for (const { block } of blocks) {
      const runs = block.targets?.[locale];
      if (!runs || runs.length === 0) continue;
      dict[block.hash] = flattenRuns(runs);
    }
    const outPath = join(outDir, `${locale}.json`);
    mkdirSync(dirname(outPath), { recursive: true });
    writeFileSync(outPath, `${JSON.stringify(dict, null, 2)}\n`);
    console.log(`Compiled ${Object.keys(dict).length} entries → ${outPath}`);
    totalCompiled += Object.keys(dict).length;
  }

  if (totalCompiled === 0) {
    console.warn('warning: no translated blocks found for any target locale');
  }
}

async function loadBlocks(input: string): Promise<{
  blocks: BlockRecord[];
  declaredTargets: string[];
}> {
  if (input === '-') return loadBlocksFromStdin();
  const stat = statSync(input);
  if (stat.isDirectory()) return loadBlocksFromKLFDir(input);
  if (extname(input).toLowerCase() === '.klf') return loadBlocksFromKLF(input);
  return loadBlocksFromKLZ(input);
}

function loadBlocksFromKLZ(path: string): {
  blocks: BlockRecord[];
  declaredTargets: string[];
} {
  const archive = readFileSync(path);
  const reader = new KlzReader(new Uint8Array(archive));
  const blocks: BlockRecord[] = [];
  for (const { block } of reader.blocks()) blocks.push({ block });
  return {
    blocks,
    declaredTargets: reader.manifest.project.targetLocales ?? [],
  };
}

function loadBlocksFromKLF(path: string): {
  blocks: BlockRecord[];
  declaredTargets: string[];
} {
  const raw = readFileSync(path, 'utf-8');
  const file = JSON.parse(raw) as File;
  const blocks: BlockRecord[] = [];
  for (const doc of file.documents ?? []) {
    for (const block of doc.blocks ?? []) blocks.push({ block });
  }
  // KLF's Project doesn't declare target locales — that lives on
  // the KLZ manifest. Infer from block.targets instead.
  return { blocks, declaredTargets: [] };
}

function loadBlocksFromKLFDir(dir: string): {
  blocks: BlockRecord[];
  declaredTargets: string[];
} {
  const blocks: BlockRecord[] = [];
  const declared = new Set<string>();
  walkKLFs(dir, (path) => {
    const res = loadBlocksFromKLF(path);
    blocks.push(...res.blocks);
    for (const l of res.declaredTargets) declared.add(l);
  });
  return { blocks, declaredTargets: Array.from(declared) };
}

function walkKLFs(dir: string, visit: (path: string) => void) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) walkKLFs(path, visit);
    else if (entry.isFile() && extname(path).toLowerCase() === '.klf') visit(path);
  }
}

async function loadBlocksFromStdin(): Promise<{
  blocks: BlockRecord[];
  declaredTargets: string[];
}> {
  const chunks: Buffer[] = [];
  for await (const chunk of process.stdin) {
    if (Buffer.isBuffer(chunk)) chunks.push(chunk);
    else if (typeof chunk === 'string') chunks.push(Buffer.from(chunk, 'utf8'));
    else chunks.push(Buffer.from(chunk as unknown as ArrayBuffer));
  }
  const text = Buffer.concat(chunks).toString('utf8');
  const blocks: BlockRecord[] = [];
  for (const line of text.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed.startsWith('{')) continue;
    const rec = JSON.parse(trimmed) as { type: string; block?: Block };
    if (rec.type === 'block' && rec.block) blocks.push({ block: rec.block });
  }
  return { blocks, declaredTargets: [] };
}

const usage = `
kapi-react compile — flatten translated blocks into runtime dictionaries.

Usage:
  kapi-react compile <input> [--locale <lang>]... [--out <dir>]

<input> can be:
  <file.klz>           a .klz archive
  <dir/>               a directory of .klf files (recursive)
  <file.klf>           a single .klf file
  -                    NDJSON block records on stdin

Options:
  --locale <lang>   Emit a dictionary for this locale (repeat for multiple).
                    If omitted, every locale present in block.targets or the
                    manifest/file's targetLocales is emitted.
  --out <dir>       Output directory (default: public/translations)
`;
