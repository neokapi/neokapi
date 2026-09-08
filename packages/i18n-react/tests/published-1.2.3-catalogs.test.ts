/**
 * The catalogs @neokapi/i18n-react 1.2.3 wrote, read by the current package.
 *
 * That build carries the install line the docs give a reader, and it extracts
 * to `.klf` under the root kind `kapi-localization-format`. Everything else
 * about the file is a current bundle. Whoever ran the walkthrough has a tree
 * of them, with the targets kapi produced beside the sources, so the current
 * package has to read one and the extractor has to converge the tree rather
 * than leave two catalogs per source behind (#2599).
 */

import { describe, expect, it, vi } from "vitest";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";

import type { Block, File } from "@neokapi/kapi-format";
import { Ext, ExtI18nReact, Kind, KindI18nReact, marshalFile } from "@neokapi/kapi-format";

import { runCompile } from "../src/commands/compile.ts";
import { runExtract } from "../src/commands/extract.ts";

const HEADING = "<h1>Welcome to the store</h1>;";

function workspace(sources: Record<string, string>): string {
  const root = mkdtempSync(join(tmpdir(), "i18n-react-published-"));
  for (const [file, code] of Object.entries(sources)) {
    const path = join(root, file);
    mkdirSync(join(path, ".."), { recursive: true });
    writeFileSync(path, code);
  }
  return root;
}

async function extractIn(root: string): Promise<void> {
  const cwd = process.cwd();
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  process.chdir(root);
  try {
    await runExtract(["--src", "src/**/*.tsx", "--out", "i18n"]);
  } finally {
    process.chdir(cwd);
    log.mockRestore();
    warn.mockRestore();
  }
}

function tree(dir: string): string[] {
  const out: string[] = [];
  const walk = (d: string) => {
    for (const entry of readdirSync(d, { withFileTypes: true })) {
      const path = join(d, entry.name);
      if (entry.isDirectory()) walk(path);
      else out.push(relative(dir, path));
    }
  };
  if (existsSync(dir)) walk(dir);
  return out.sort();
}

/**
 * Rewrite every catalog under `dir` the way 1.2.3 wrote it: the older suffix
 * and the older root kind, byte-identical otherwise. That equality is the
 * premise the whole fix rests on, and it is asserted directly below.
 */
function rewriteAsPublished(dir: string): void {
  for (const rel of tree(dir)) {
    if (!rel.endsWith(Ext)) continue;
    const path = join(dir, rel);
    const raw = readFileSync(path, "utf8");
    writeFileSync(
      join(dir, rel.slice(0, -Ext.length) + ExtI18nReact),
      raw.replace(`"kind": "${Kind}"`, `"kind": "${KindI18nReact}"`),
    );
    writeFileSync(path, raw);
  }
}

function bundle(path: string): File {
  return JSON.parse(readFileSync(path, "utf8")) as File;
}

async function compileDictionary(input: string, locale: string, out: string): Promise<string> {
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  try {
    await runCompile([input, "--locale", locale, "--out", out]);
  } finally {
    log.mockRestore();
    warn.mockRestore();
  }
  return readFileSync(join(out, `${locale}.json`), "utf8");
}

describe("catalogs written by @neokapi/i18n-react 1.2.3", () => {
  it("differ from a current catalog in the suffix and the root kind alone", async () => {
    const root = workspace({ "src/Page.tsx": HEADING });
    await extractIn(root);
    const current = readFileSync(join(root, "i18n/src/Page.kbf.json"), "utf8");

    rewriteAsPublished(join(root, "i18n"));
    const published = readFileSync(join(root, "i18n/src/Page.klf"), "utf8");

    expect(published).not.toEqual(current);
    expect(published.replace(KindI18nReact, Kind)).toEqual(current);
  });

  it("compile reads their targets, so a tree translated before the upgrade still ships", async () => {
    const root = workspace({ "src/Page.tsx": HEADING });
    await extractIn(root);

    // The target tree kapi writes beside the source, under the suffix it read.
    const targets = join(root, "i18n-nb");
    mkdirSync(join(targets, "src"), { recursive: true });
    const file = bundle(join(root, "i18n/src/Page.kbf.json"));
    for (const doc of file.documents) {
      for (const block of doc.blocks as Block[]) {
        block.targets = { nb: [{ text: "Velkommen til butikken" }] };
      }
    }
    writeFileSync(
      join(targets, "src/Page.klf"),
      new TextDecoder()
        .decode(marshalFile(file))
        .replace(`"kind": "${Kind}"`, `"kind": "${KindI18nReact}"`),
    );

    const dict = JSON.parse(await compileDictionary(targets, "nb", join(root, "out"))) as Record<
      string,
      string
    >;
    expect(Object.values(dict)).toContain("Velkommen til butikken");
  });

  it("extract replaces the catalog it owns rather than leaving both spellings", async () => {
    const root = workspace({ "src/Page.tsx": HEADING, "src/nested/Deep.tsx": "<p>Deep</p>;" });
    await extractIn(root);
    rewriteAsPublished(join(root, "i18n"));
    expect(tree(join(root, "i18n"))).toEqual([
      "src/Page.kbf.json",
      "src/Page.klf",
      "src/nested/Deep.kbf.json",
      "src/nested/Deep.klf",
    ]);

    await extractIn(root);
    expect(tree(join(root, "i18n"))).toEqual(["src/Page.kbf.json", "src/nested/Deep.kbf.json"]);
  });

  it("leaves a per-locale target tree alone, which is where the translations live", async () => {
    const root = workspace({ "src/Page.tsx": HEADING });
    await extractIn(root);

    // kapi writes targets under i18n/{lang}/, recording the same document path
    // from a different position. Ownership is decided by position, so an
    // upgrade must not sweep those away with the stale sources.
    const file = bundle(join(root, "i18n/src/Page.kbf.json"));
    mkdirSync(join(root, "i18n/nb"), { recursive: true });
    writeFileSync(
      join(root, "i18n/nb/Page.klf"),
      new TextDecoder()
        .decode(marshalFile(file))
        .replace(`"kind": "${Kind}"`, `"kind": "${KindI18nReact}"`),
    );

    await extractIn(root);
    expect(tree(join(root, "i18n"))).toEqual(["nb/Page.klf", "src/Page.kbf.json"]);
  });
});
