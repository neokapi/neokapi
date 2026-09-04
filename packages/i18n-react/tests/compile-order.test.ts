/**
 * The key order of a compiled dictionary is a function of the catalog paths
 * and the block order within each, never of the order a filesystem lists a
 * directory in: two checkouts of the same catalogs write the same bytes.
 */

import { describe, expect, it, vi } from "vitest";
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import type { Block } from "@neokapi/kapi-format";
import { marshalFile, newFile } from "@neokapi/kapi-format";

import { runCompile } from "../src/commands/compile.ts";

function block(hash: string, text: string): Block {
  return {
    id: hash,
    hash,
    translatable: true,
    type: "jsx:element",
    source: [{ text }],
    targets: { nb: [{ text: `${text} (nb)` }] },
    placeholders: [],
    properties: { file: "X.tsx", line: 1, component: "X", jsxPath: "p", element: "p" },
  } as Block;
}

function catalog(dir: string, rel: string, blocks: Block[]) {
  const path = join(dir, rel);
  mkdirSync(join(path, ".."), { recursive: true });
  writeFileSync(
    path,
    marshalFile(
      newFile({
        generator: { id: "test", version: "1" },
        project: { id: "order", sourceLocale: "en" },
        documents: [{ id: rel, documentType: "jsx", path: rel, blocks }],
      }),
    ),
  );
}

async function compile(dir: string): Promise<string[]> {
  const out = join(dir, "out");
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  try {
    await runCompile([join(dir, "i18n"), "--locale", "nb", "--out", out]);
  } finally {
    log.mockRestore();
  }
  return Object.keys(JSON.parse(readFileSync(join(out, "nb.json"), "utf8")) as object);
}

describe("compile walks catalogs in byte order", () => {
  it("orders keys by catalog path, depth first, then by block", async () => {
    const dir = mkdtempSync(join(tmpdir(), "compile-order-"));
    // Written in an order no sort would produce; uppercase sorts before
    // lowercase in byte order, and a directory is walked where its name falls.
    catalog(dir, "i18n/src/b/Late.kbf.json", [block("kLate", "late")]);
    catalog(dir, "i18n/src/a.kbf.json", [block("kA1", "a one"), block("kA2", "a two")]);
    catalog(dir, "i18n/src/Upper.kbf.json", [block("kUpper", "upper")]);
    catalog(dir, "i18n/src/b/Early.kbf.json", [block("kEarly", "early")]);
    catalog(dir, "i18n/root.kbf.json", [block("kRoot", "root")]);

    expect(await compile(dir)).toEqual(["kRoot", "kUpper", "kA1", "kA2", "kEarly", "kLate"]);
  });

  it("writes the same bytes whichever order the catalogs were created in", async () => {
    const a = mkdtempSync(join(tmpdir(), "compile-order-a-"));
    const b = mkdtempSync(join(tmpdir(), "compile-order-b-"));
    const files: [string, Block[]][] = [
      ["i18n/src/one.kbf.json", [block("k1", "one")]],
      ["i18n/src/two.kbf.json", [block("k2", "two")]],
      ["i18n/src/three.kbf.json", [block("k3", "three")]],
    ];
    for (const [rel, blocks] of files) catalog(a, rel, blocks);
    for (const [rel, blocks] of [...files].reverse()) catalog(b, rel, blocks);

    expect(await compile(a)).toEqual(await compile(b));
    expect(readFileSync(join(a, "out/nb.json"), "utf8")).toBe(
      readFileSync(join(b, "out/nb.json"), "utf8"),
    );
  });
});
