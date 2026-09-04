/**
 * A re-extract keeps the targets of every block whose content hash is
 * unchanged, in the same catalog.
 *
 * The in-place layout puts each block's targets beside its source in the one
 * `.kbf.json` the extract writes (`kapi pseudo-translate i18n/` with no `-o`).
 * An extract that rewrote each catalog from the source alone threw those
 * targets away, so the workflow the README documents lost its translations on
 * the second run (#2406). Blocks are matched by hash, never by id: the id spells
 * a line and a position, which move with every edit, while the hash names the
 * content a target translates.
 */

import { describe, expect, it, vi } from "vitest";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import type { Block, File, Run, TargetOrigin } from "@neokapi/kapi-format";
import { marshalFile } from "@neokapi/kapi-format";

import { runCompile } from "../src/commands/compile.ts";
import { runExtract } from "../src/commands/extract.ts";

type Sources = Record<string, string>;

function workspace(sources: Sources): string {
  const root = mkdtempSync(join(tmpdir(), "i18n-react-targets-"));
  write(root, sources);
  return root;
}

function write(root: string, sources: Sources) {
  for (const [file, code] of Object.entries(sources)) {
    const path = join(root, file);
    mkdirSync(join(path, ".."), { recursive: true });
    writeFileSync(path, code);
  }
}

async function extractIn(root: string, args: string[] = []): Promise<string[]> {
  const cwd = process.cwd();
  const lines: string[] = [];
  const log = vi.spyOn(console, "log").mockImplementation((...a: unknown[]) => {
    lines.push(a.map(String).join(" "));
  });
  const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  process.chdir(root);
  try {
    await runExtract(["--src", "src/**/*.tsx", "--out", "i18n", ...args]);
  } finally {
    process.chdir(cwd);
    log.mockRestore();
    warn.mockRestore();
  }
  return lines;
}

function bundle(path: string): File {
  return JSON.parse(readFileSync(path, "utf8")) as File;
}

function blocks(path: string): Block[] {
  return bundle(path).documents.flatMap((d) => d.blocks as Block[]);
}

function text(runs: Run[] | undefined): string {
  return (runs ?? []).map((r) => ("text" in r ? r.text : "")).join("");
}

/**
 * What kapi does to a catalog in place: every block gains a target for
 * `locale`, stamped with how it was produced, and the file is written back at
 * the same position.
 */
function translateInPlace(
  path: string,
  locale: string,
  render: (source: string) => string,
  origin?: TargetOrigin,
) {
  const file = bundle(path);
  for (const doc of file.documents) {
    for (const block of doc.blocks as Block[]) {
      block.targets = {
        ...(block.targets ?? {}),
        [locale]: [{ text: render(text(block.source)) }],
      };
      if (origin) block.targetOrigins = { ...(block.targetOrigins ?? {}), [locale]: origin };
    }
  }
  writeFileSync(path, marshalFile(file));
}

const upper = (s: string) => s.toUpperCase();
const PSEUDO: TargetOrigin = {
  kind: "mt",
  tool: "pseudo-translate",
  timestamp: "2026-09-04T00:00:00Z",
};

describe("extract keeps the targets of unchanged blocks", () => {
  it("preserves every locale, and its provenance, for a block whose hash is unchanged", async () => {
    const root = workspace({ "src/App.tsx": "<h1>Welcome</h1>;" });
    await extractIn(root);
    const catalog = join(root, "i18n/src/App.kbf.json");
    translateInPlace(catalog, "qps", upper, PSEUDO);
    translateInPlace(catalog, "fr", () => "Bienvenue", { kind: "ai", engine: "claude" });
    const before = readFileSync(catalog, "utf8");

    const lines = await extractIn(root);

    expect(readFileSync(catalog, "utf8")).toBe(before);
    const [block] = blocks(catalog);
    expect(text(block.targets?.qps)).toBe("WELCOME");
    expect(text(block.targets?.fr)).toBe("Bienvenue");
    expect(block.targetOrigins).toEqual({
      fr: { kind: "ai", engine: "claude" },
      qps: PSEUDO,
    });
    expect(lines).toContain("Kept the targets of 1 unchanged block(s) (fr, qps)");
  });

  it("adds a new block without a target and keeps the rest", async () => {
    const root = workspace({ "src/App.tsx": "<><h1>Welcome</h1><p>Old</p></>;" });
    await extractIn(root);
    const catalog = join(root, "i18n/src/App.kbf.json");
    translateInPlace(catalog, "qps", upper, PSEUDO);

    write(root, { "src/App.tsx": "<><p>Fresh</p><h1>Welcome</h1><p>Old</p></>;" });
    await extractIn(root);

    const byText = new Map(blocks(catalog).map((b) => [text(b.source), b]));
    expect([...byText.keys()]).toEqual(["Fresh", "Welcome", "Old"]);
    expect(byText.get("Fresh")?.targets).toBeUndefined();
    expect(byText.get("Fresh")?.targetOrigins).toBeUndefined();
    expect(text(byText.get("Welcome")?.targets?.qps)).toBe("WELCOME");
    expect(text(byText.get("Old")?.targets?.qps)).toBe("OLD");
    // The two kept blocks moved down a line, so their ids changed; the
    // targets followed the content, not the position.
    expect(byText.get("Welcome")?.id).toBe("src/App.tsx:1:1");
  });

  it("drops the target of a block whose text changed", async () => {
    const root = workspace({ "src/App.tsx": "<><h1>Welcome</h1><p>Old</p></>;" });
    await extractIn(root);
    const catalog = join(root, "i18n/src/App.kbf.json");
    translateInPlace(catalog, "qps", upper, PSEUDO);

    write(root, { "src/App.tsx": "<><h1>Welcome back</h1><p>Old</p></>;" });
    const lines = await extractIn(root);

    const byText = new Map(blocks(catalog).map((b) => [text(b.source), b]));
    expect(byText.get("Welcome back")?.targets).toBeUndefined();
    expect(byText.get("Welcome back")?.targetOrigins).toBeUndefined();
    expect(text(byText.get("Old")?.targets?.qps)).toBe("OLD");
    expect(lines).toContain("Kept the targets of 1 unchanged block(s) (qps)");
    expect(lines).toContain("Dropped the targets of 1 block(s) whose source changed or is gone");
  });

  it("drops the target of a block whose element changed", async () => {
    // Same words under another tag hash apart: a heading's translation is
    // no answer for a button.
    const root = workspace({ "src/App.tsx": "<h1>Save</h1>;" });
    await extractIn(root);
    const catalog = join(root, "i18n/src/App.kbf.json");
    translateInPlace(catalog, "qps", upper, PSEUDO);

    write(root, { "src/App.tsx": "<button>Save</button>;" });
    await extractIn(root);

    const [block] = blocks(catalog);
    expect(block.properties.element).toBe("button");
    expect(block.targets).toBeUndefined();
  });

  it("removes a block's targets with the block, and a catalog's with the catalog", async () => {
    const root = workspace({
      "src/App.tsx": "<><h1>Welcome</h1><p>Old</p></>;",
      "src/Gone.tsx": "<p>Gone paragraph</p>;",
    });
    await extractIn(root);
    const app = join(root, "i18n/src/App.kbf.json");
    translateInPlace(app, "qps", upper, PSEUDO);
    translateInPlace(join(root, "i18n/src/Gone.kbf.json"), "qps", upper, PSEUDO);

    write(root, { "src/App.tsx": "<h1>Welcome</h1>;" });
    rmSync(join(root, "src/Gone.tsx"));
    await extractIn(root);

    expect(existsSync(join(root, "i18n/src/Gone.kbf.json"))).toBe(false);
    const all = blocks(app);
    expect(all).toHaveLength(1);
    expect(text(all[0].targets?.qps)).toBe("WELCOME");
  });

  it("keeps the one target of a string that appears twice in a file", async () => {
    // Two identical elements are one block: the extractor dedupes by hash
    // within a document, and the dictionary keys on the hash. The target
    // follows the block however many times the string is rendered.
    const root = workspace({ "src/App.tsx": "<><button>Save</button><button>Save</button></>;" });
    await extractIn(root);
    const catalog = join(root, "i18n/src/App.kbf.json");
    expect(blocks(catalog)).toHaveLength(1);
    translateInPlace(catalog, "qps", upper, PSEUDO);

    write(root, {
      "src/App.tsx": "<><button>Save</button><p>Between</p><button>Save</button></>;",
    });
    await extractIn(root);

    const saves = blocks(catalog).filter((b) => text(b.source) === "Save");
    expect(saves).toHaveLength(1);
    expect(text(saves[0].targets?.qps)).toBe("SAVE");
  });

  it("is stable: re-extracting an unchanged, translated tree rewrites the same bytes", async () => {
    const root = workspace({
      "src/App.tsx": "<><h1>Welcome, {name}</h1><p>Read <a href='/x'>more</a></p></>;",
    });
    await extractIn(root);
    const catalog = join(root, "i18n/src/App.kbf.json");
    translateInPlace(catalog, "qps", upper, PSEUDO);
    const once = readFileSync(catalog, "utf8");

    await extractIn(root);
    const twice = readFileSync(catalog, "utf8");
    await extractIn(root);

    expect(twice).toBe(once);
    expect(readFileSync(catalog, "utf8")).toBe(once);
  });

  it("carries a target the compiler can ship", async () => {
    const root = workspace({ "src/App.tsx": "<><h1>Welcome</h1><p>Old</p></>;" });
    await extractIn(root);
    const catalog = join(root, "i18n/src/App.kbf.json");
    translateInPlace(catalog, "qps", upper, PSEUDO);
    write(root, { "src/App.tsx": "<><p>Fresh</p><h1>Welcome</h1><p>Old</p></>;" });
    await extractIn(root);

    const out = join(root, "out");
    const log = vi.spyOn(console, "log").mockImplementation(() => {});
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    try {
      await runCompile([join(root, "i18n"), "--locale", "qps", "--out", out]);
    } finally {
      log.mockRestore();
      warn.mockRestore();
    }
    const dict = JSON.parse(readFileSync(join(out, "qps.json"), "utf8")) as Record<string, string>;
    const byText = new Map(blocks(catalog).map((b) => [text(b.source), b.hash]));
    expect(dict).toEqual({
      [byText.get("Welcome")!]: "WELCOME",
      [byText.get("Old")!]: "OLD",
    });
  });

  it("overwrites a catalog it cannot read as a bundle", async () => {
    const root = workspace({ "src/App.tsx": "<h1>Welcome</h1>;" });
    await extractIn(root);
    const catalog = join(root, "i18n/src/App.kbf.json");
    writeFileSync(catalog, "not json");

    await extractIn(root);

    const [block] = blocks(catalog);
    expect(text(block.source)).toBe("Welcome");
    expect(block.targets).toBeUndefined();
  });

  it("leaves the per-locale layout's targets where kapi wrote them", async () => {
    // `kapi init --framework neokapi-i18n` scaffolds targets under i18n/{lang}/
    // from sources under i18n/src/. The extract writes only the latter, so a
    // re-extract touches nothing under the former, changed source or not.
    const root = workspace({ "src/App.tsx": "<h1>Welcome</h1>;" });
    await extractIn(root);
    const source = bundle(join(root, "i18n/src/App.kbf.json"));
    const target = join(root, "i18n/nb/src/App.kbf.json");
    mkdirSync(join(target, ".."), { recursive: true });
    writeFileSync(target, marshalFile(source));
    translateInPlace(target, "nb", () => "Velkommen");
    const before = readFileSync(target, "utf8");

    write(root, { "src/App.tsx": "<h1>Welcome back</h1>;" });
    await extractIn(root);

    expect(readFileSync(target, "utf8")).toBe(before);
    expect(blocks(join(root, "i18n/src/App.kbf.json"))[0].targets).toBeUndefined();
  });
});
