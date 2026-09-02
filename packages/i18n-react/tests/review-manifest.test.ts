import { mkdtempSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, it } from "vitest";
import type { Block, File } from "@neokapi/kapi-format";
import { newFile, marshalFile } from "@neokapi/kapi-format";

import { buildReviewManifest, type ReviewManifest } from "../src/review/manifest.ts";
import { runCompile } from "../src/commands/compile.ts";

function tempDir(prefix: string) {
  return mkdtempSync(join(tmpdir(), `${prefix}-`));
}

function block(overrides: Partial<Block> = {}): Block {
  return {
    id: "welcome",
    hash: "h-welcome",
    translatable: true,
    type: "jsx:element",
    source: [{ text: "Welcome" }],
    targets: {},
    placeholders: [],
    properties: { file: "App.tsx", line: 3, component: "App", jsxPath: "h1", element: "h1" },
    ...overrides,
  } as Block;
}

function fileWith(b: Block): File {
  return newFile({
    generator: { id: "test", version: "1" },
    project: { id: "review-test", sourceLocale: "en" },
    documents: [{ id: "App", documentType: "jsx", path: "App.tsx", blocks: [b] }],
  });
}

describe("buildReviewManifest", () => {
  it("merges source + per-locale targets across the source catalog and i18n-<locale> trees", () => {
    const dir = tempDir("review");
    // Source catalog: source only, carries the properties.
    mkdirSync(join(dir, "i18n"), { recursive: true });
    writeFileSync(join(dir, "i18n", "App.kbf.json"), marshalFile(fileWith(block())));
    // Two locale trees, each with just its own target.
    mkdirSync(join(dir, "i18n-de"), { recursive: true });
    writeFileSync(
      join(dir, "i18n-de", "App.kbf.json"),
      marshalFile(fileWith(block({ targets: { de: [{ text: "Willkommen" }] } }))),
    );
    mkdirSync(join(dir, "i18n-fr"), { recursive: true });
    writeFileSync(
      join(dir, "i18n-fr", "App.kbf.json"),
      marshalFile(fileWith(block({ targets: { fr: [{ text: "Bienvenue" }] } }))),
    );

    const manifest = buildReviewManifest([
      join(dir, "i18n"),
      join(dir, "i18n-de"),
      join(dir, "i18n-fr"),
    ]);

    expect(manifest["h-welcome"]).toEqual({
      source: "Welcome",
      targets: { de: "Willkommen", fr: "Bienvenue" },
      properties: { file: "App.tsx", line: 3, component: "App", element: "h1", locNote: undefined },
      annotations: [],
    });
  });

  it("attaches .overlays.jsonl term/check annotations to the matching block", () => {
    const dir = tempDir("review-ann");
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, "App.kbf.json"), marshalFile(fileWith(block())));
    // A stand-off annotation file referencing the block hash.
    const kbfl =
      JSON.stringify({ type: "header", annotationType: "kapi/term" }) +
      "\n" +
      JSON.stringify({
        type: "annotation",
        id: "t1",
        block: "h-welcome",
        anchor: { kind: "block" },
        data: { term: "Welcome", message: "keep on brand" },
      }) +
      "\n";
    writeFileSync(join(dir, "App.overlays.jsonl"), kbfl);

    const manifest = buildReviewManifest([dir]);
    expect(manifest["h-welcome"].annotations).toEqual([{ type: "term", summary: "keep on brand" }]);
  });
});

describe("runCompile --review", () => {
  it("emits review.json alongside the per-locale dictionaries", async () => {
    const dir = tempDir("compile-review");
    const outDir = join(dir, "out");
    mkdirSync(join(dir, "i18n"), { recursive: true });
    writeFileSync(join(dir, "i18n", "App.kbf.json"), marshalFile(fileWith(block())));
    mkdirSync(join(dir, "i18n-de"), { recursive: true });
    writeFileSync(
      join(dir, "i18n-de", "App.kbf.json"),
      marshalFile(fileWith(block({ targets: { de: [{ text: "Willkommen" }] } }))),
    );

    await runCompile([join(dir, "i18n"), join(dir, "i18n-de"), "--out", outDir, "--review"]);

    const manifest = JSON.parse(
      readFileSync(join(outDir, "review.json"), "utf-8"),
    ) as ReviewManifest;
    expect(manifest["h-welcome"].source).toBe("Welcome");
    expect(manifest["h-welcome"].targets).toEqual({ de: "Willkommen" });
    // And the ordinary de.json dictionary is still produced.
    expect(JSON.parse(readFileSync(join(outDir, "de.json"), "utf-8"))).toEqual({
      "h-welcome": "Willkommen",
    });
  });
});
