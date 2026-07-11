/**
 * kapi-react explain — print the translatability decision for every
 * JSX element in a file (or glob): which W3C rule classified it, why
 * it was or wasn't extracted, and the hash it got.
 *
 * This makes the ITS-grounded extraction auditable: "why did the
 * compiler translate this?" has a printable answer.
 */

import { readFileSync } from "node:fs";
import { glob } from "node:fs/promises";
import { relative } from "node:path";

import { extractDocument, type ExplainDecision } from "../extract/walker.ts";
import type { PluginOptions } from "../types.ts";

type ExplainConfig = Pick<PluginOptions, "componentMap" | "rules">;

const OUTCOME_LABEL: Record<ExplainDecision["outcome"], string> = {
  extracted: "extracted",
  "extracted-promoted": "extracted (container promoted: direct text + inline children)",
  "skipped-translate-no": 'skipped — translate="no" on self or ancestor',
  "skipped-not-translatable": "skipped — classified non-translatable",
  "skipped-no-text": "skipped — no translator-editable text",
  "skipped-block-children": "skipped — has block-level children (they extract separately)",
  "skipped-empty": "skipped — empty after whitespace collapse",
};

export async function runExplain(args: string[]): Promise<void> {
  const opts = parseArgs(args);
  if (opts.help || opts.patterns.length === 0) {
    console.log(usage);
    if (!opts.help) process.exitCode = 1;
    return;
  }
  const config = loadConfig(opts.configPath);

  const files = new Set<string>();
  for (const pattern of opts.patterns) {
    for await (const f of glob(pattern)) files.add(f);
  }
  if (files.size === 0) {
    console.error(`explain: no files match ${JSON.stringify(opts.patterns)}`);
    process.exitCode = 1;
    return;
  }

  for (const file of [...files].sort()) {
    const decisions: ExplainDecision[] = [];
    const filename = relative(process.cwd(), file);
    extractDocument(readFileSync(file, "utf-8"), {
      filename,
      ...config,
      onDecision: (d) => decisions.push(d),
    });
    if (decisions.length === 0) continue;

    console.log(`\n${filename}`);
    for (const d of decisions.sort((a, b) => a.line - b.line)) {
      if (opts.onlyExtracted && !d.outcome.startsWith("extracted")) continue;
      const tagLabel = d.htmlElement === d.tag ? `<${d.tag}>` : `<${d.tag}> → ${d.htmlElement}`;
      const classLabel =
        d.classification === "yes"
          ? "translatable"
          : d.classification === "no"
            ? "non-translatable"
            : "container";
      const hash = d.hash ? `  hash=${d.hash}` : "";
      const note = d.locNote ? `  note="${d.locNote}"` : "";
      console.log(
        `  L${String(d.line).padEnd(4)} ${tagLabel.padEnd(28)} [${classLabel}] ${OUTCOME_LABEL[d.outcome]}${hash}${note}`,
      );
      for (const [attr, attrHash] of Object.entries(d.attributes)) {
        console.log(
          `  ${"".padEnd(5)} ${`  ↳ ${attr}`.padEnd(28)} [attribute] extracted  hash=${attrHash}`,
        );
      }
    }
  }
}

interface ExplainArgs {
  patterns: string[];
  configPath: string | null;
  onlyExtracted: boolean;
  help: boolean;
}

function parseArgs(args: string[]): ExplainArgs {
  const parsed: ExplainArgs = {
    patterns: [],
    configPath: null,
    onlyExtracted: false,
    help: false,
  };
  for (let i = 0; i < args.length; i++) {
    const flag = args[i];
    switch (flag) {
      case "--help":
      case "-h":
        parsed.help = true;
        return parsed;
      case "--config":
        if (args[i + 1]) parsed.configPath = args[++i];
        break;
      case "--extracted":
        parsed.onlyExtracted = true;
        break;
      default:
        if (flag.startsWith("--")) console.warn(`unknown flag: ${flag}`);
        else parsed.patterns.push(flag);
    }
  }
  return parsed;
}

function loadConfig(path: string | null): ExplainConfig {
  if (!path) return {};
  try {
    return JSON.parse(readFileSync(path, "utf-8")) as ExplainConfig;
  } catch (e) {
    console.error(`Failed to load config from ${path}:`, e);
    process.exit(1);
  }
}

const usage = `
kapi-react explain — show the translatability decision for every JSX
element: the W3C ITS classification, why it was or wasn't extracted,
and the hash it received.

Usage:
  kapi-react explain <file-or-glob>... [options]

Options:
  --config <path>   Config file with componentMap, rules, …
  --extracted       Only show elements that produced a block
`;
