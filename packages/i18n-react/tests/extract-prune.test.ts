/**
 * The catalog tree under --out is a mirror of the sources scanned, and it stays
 * one across runs.
 *
 * A catalog for a component that was deleted or moved names strings nobody can
 * reach. Left under --out it is translated and compiled like every other, so
 * the runtime dictionary ships them, and a checkout that has run the extract
 * for weeks regenerates different bytes from a clean checkout of the same
 * source (#2401). The mirror is kept by position: a bundle sitting exactly
 * where this run would write the document it records is the extractor's own.
 */

import { describe, expect, it, vi } from "vitest";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";

import type { Block, File } from "@neokapi/kapi-format";
import { marshalFile } from "@neokapi/kapi-format";

import { runCompile } from "../src/commands/compile.ts";
import { runExtract } from "../src/commands/extract.ts";

type Sources = Record<string, string>;

/** A package with the given source files under src/. */
function workspace(sources: Sources): string {
  const root = mkdtempSync(join(tmpdir(), "i18n-react-prune-"));
  for (const [file, code] of Object.entries(sources)) {
    const path = join(root, file);
    mkdirSync(join(path, ".."), { recursive: true });
    writeFileSync(path, code);
  }
  return root;
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

/** Every file under `dir`, as a sorted list of relative paths. */
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

/** The tree's contents, path to bytes. */
function snapshot(dir: string): Map<string, string> {
  return new Map(tree(dir).map((p) => [p, readFileSync(join(dir, p), "utf8")]));
}

function bundle(path: string): File {
  return JSON.parse(readFileSync(path, "utf8")) as File;
}

/**
 * What the pipeline does between extract and compile, reduced to its shape:
 * every catalog in `src` gets a target for `locale` that is its own source,
 * written to the same position under `out`. The target tree is derived from
 * the source tree in full on every run, as `make l10n-pseudo` derives it.
 */
function mirrorAsTargets(src: string, out: string, locale: string) {
  rmSync(out, { recursive: true, force: true });
  for (const rel of tree(src)) {
    const file = bundle(join(src, rel));
    for (const doc of file.documents) {
      for (const block of doc.blocks as Block[]) block.targets = { [locale]: block.source };
    }
    const path = join(out, rel);
    mkdirSync(join(path, ".."), { recursive: true });
    writeFileSync(path, marshalFile(file));
  }
}

async function compileDictionary(root: string, locale: string): Promise<string> {
  mirrorAsTargets(join(root, "i18n"), join(root, `i18n-${locale}`), locale);
  const out = join(root, "out");
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  try {
    await runCompile([join(root, `i18n-${locale}`), "--locale", locale, "--out", out]);
  } finally {
    log.mockRestore();
    warn.mockRestore();
  }
  return readFileSync(join(out, `${locale}.json`), "utf8");
}

const KEEP = "<h1>Kept heading</h1>;";
const GONE = "<p>Gone paragraph</p>;";
const DEEP = "<button>Deep button</button>;";

describe("extract keeps --out a mirror of the sources scanned", () => {
  it("regenerates over a stale checkout the same bytes as over a clean one", async () => {
    // A checkout that extracted, then lost a source.
    const stale = workspace({
      "src/Keep.tsx": KEEP,
      "src/Gone.tsx": GONE,
      "src/nested/Deep.tsx": DEEP,
    });
    await extractIn(stale);
    expect(tree(join(stale, "i18n"))).toEqual([
      "src/Gone.kbf.json",
      "src/Keep.kbf.json",
      "src/nested/Deep.kbf.json",
    ]);
    rmSync(join(stale, "src/Gone.tsx"));
    rmSync(join(stale, "src/nested"), { recursive: true });
    await extractIn(stale);

    // A clean checkout of the same source.
    const clean = workspace({ "src/Keep.tsx": KEEP });
    await extractIn(clean);

    expect(snapshot(join(stale, "i18n"))).toEqual(snapshot(join(clean, "i18n")));
    expect(tree(join(stale, "i18n"))).toEqual(["src/Keep.kbf.json"]);
  });

  it("removes the catalog of a dropped source and the directory that leaves empty", async () => {
    const root = workspace({ "src/Keep.tsx": KEEP, "src/nested/Deep.tsx": DEEP });
    await extractIn(root);
    rmSync(join(root, "src/nested"), { recursive: true });

    const lines = await extractIn(root);

    expect(existsSync(join(root, "i18n/src/nested"))).toBe(false);
    expect(existsSync(join(root, "i18n/src/Keep.kbf.json"))).toBe(true);
    expect(lines.some((l) => l.startsWith("Removed 1 stale catalog"))).toBe(true);
  });

  it("removes the catalog of a source the scan no longer includes", async () => {
    const root = workspace({ "src/Keep.tsx": KEEP, "src/Gone.tsx": GONE });
    await extractIn(root);

    await extractIn(root, ["--ignore", "src/Gone.tsx"]);

    expect(tree(join(root, "i18n"))).toEqual(["src/Keep.kbf.json"]);
  });

  it("removes a catalog written under an earlier --source-root", async () => {
    const root = workspace({ "src/Keep.tsx": KEEP });
    await extractIn(root);
    expect(tree(join(root, "i18n"))).toEqual(["src/Keep.kbf.json"]);

    // Declaring the root re-keys the surface: every catalog moves, and the one
    // at the old position records a document this run would write there too.
    const parent = mkdtempSync(join(tmpdir(), "i18n-react-prune-parent-"));
    const nested = join(parent, "pkg");
    mkdirSync(nested);
    rmSync(nested, { recursive: true });
    mkdirSync(join(nested, "src"), { recursive: true });
    writeFileSync(join(nested, "src/Keep.tsx"), KEEP);
    await extractIn(nested);
    await extractIn(nested, ["--source-root", ".."]);

    expect(tree(join(nested, "i18n"))).toEqual(["pkg/src/Keep.kbf.json"]);
  });

  it("leaves per-locale targets kapi writes under --out alone", async () => {
    const root = workspace({ "src/Keep.tsx": KEEP });
    await extractIn(root);

    // The layout `kapi init --framework neokapi-i18n` scaffolds: the target
    // records the same document path and sits under i18n/{lang}/.
    const source = bundle(join(root, "i18n/src/Keep.kbf.json"));
    const target = join(root, "i18n/nb/src/Keep.kbf.json");
    mkdirSync(join(target, ".."), { recursive: true });
    writeFileSync(target, marshalFile(source));

    await extractIn(root);

    expect(tree(join(root, "i18n"))).toEqual(["nb/src/Keep.kbf.json", "src/Keep.kbf.json"]);
  });

  it("leaves a file at its own position that is not a bundle alone", async () => {
    const root = workspace({ "src/Keep.tsx": KEEP });
    await extractIn(root);
    writeFileSync(join(root, "i18n/src/Notes.kbf.json"), "not json");
    writeFileSync(join(root, "i18n/src/Empty.kbf.json"), "{}");
    writeFileSync(join(root, "i18n/README.md"), "hand-written");

    await extractIn(root);

    expect(tree(join(root, "i18n"))).toEqual([
      "README.md",
      "src/Empty.kbf.json",
      "src/Keep.kbf.json",
      "src/Notes.kbf.json",
    ]);
  });

  it("prunes nothing when the scan matches no file", async () => {
    const root = workspace({ "src/Keep.tsx": KEEP });
    await extractIn(root);

    await extractIn(root, ["--ignore", "src/**"]);

    expect(tree(join(root, "i18n"))).toEqual(["src/Keep.kbf.json"]);
  });

  // The drift that hit the desktop pseudo-locale catalog: a component moved to
  // another file, its strings re-keyed under the new element, and the old
  // file's catalog stayed behind to be translated and compiled beside the new.
  it("compiles one key per string after a component moves between files", async () => {
    const before: Sources = {
      "src/FlowsPage.tsx": "<p>Copy to edit</p>;",
      "src/Other.tsx": "<h1>Other</h1>;",
    };
    const after: Sources = {
      "src/FlowCard.tsx": "<button>Copy to edit</button>;",
      "src/Other.tsx": "<h1>Other</h1>;",
    };

    const stale = workspace(before);
    await extractIn(stale);
    await compileDictionary(stale, "qps");
    rmSync(join(stale, "src/FlowsPage.tsx"));
    writeFileSync(join(stale, "src/FlowCard.tsx"), after["src/FlowCard.tsx"]);
    await extractIn(stale);

    const clean = workspace(after);
    await extractIn(clean);

    const dirty = await compileDictionary(stale, "qps");
    const fresh = await compileDictionary(clean, "qps");
    expect(dirty).toBe(fresh);
    expect(Object.keys(JSON.parse(fresh) as Record<string, string>)).toHaveLength(2);
  });
});
