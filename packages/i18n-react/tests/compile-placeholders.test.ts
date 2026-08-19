/**
 * A runtime dictionary ships only translations that still have their holes.
 *
 * The runtime falls back to source for a key it cannot find, so leaving a
 * target out costs a sentence its translation. Shipping one that lost its
 * placeholders costs the sentence its count, its name or its link — and says
 * nothing about having done so. The dogfood loop produced 1221 of these in one
 * night, every one of them a string a reader would have been handed with the
 * value missing.
 */
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, it, vi } from "vitest";
import type { Block, File } from "@neokapi/kapi-format";
import { newFile, marshalFile } from "@neokapi/kapi-format";

import { runCompile } from "../src/commands/compile.ts";

function block(overrides: Partial<Block> = {}): Block {
  return {
    id: "b",
    hash: "h",
    translatable: true,
    type: "jsx:element",
    source: [{ text: "Welcome" }],
    targets: {},
    placeholders: [],
    properties: { file: "App.tsx", line: 3, component: "App", jsxPath: "h1", element: "h1" },
    ...overrides,
  } as Block;
}

function fileWith(blocks: Block[]): File {
  return newFile({
    generator: { id: "test", version: "1" },
    project: { id: "compile-test", sourceLocale: "en" },
    documents: [{ id: "App", documentType: "jsx", path: "App.tsx", blocks }],
  });
}

async function compile(blocks: Block[]): Promise<Record<string, string>> {
  const dir = mkdtempSync(join(tmpdir(), "compile-"));
  mkdirSync(join(dir, "i18n"), { recursive: true });
  writeFileSync(join(dir, "i18n", "App.kbf.json"), marshalFile(fileWith(blocks)));
  const out = join(dir, "out");
  const quiet = vi.spyOn(console, "log").mockImplementation(() => {});
  const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  try {
    await runCompile([join(dir, "i18n"), "--locale", "nb", "--out", out]);
  } finally {
    quiet.mockRestore();
    warn.mockRestore();
  }
  return JSON.parse(readFileSync(join(out, "nb.json"), "utf8")) as Record<string, string>;
}

describe("compile — a target keeps its source's placeholders or it does not ship", () => {
  it("ships a translation that carries them", async () => {
    const dict = await compile([
      block({
        hash: "kept",
        source: [{ text: "Reset on " }, { ph: { id: "1", equiv: "date" } }],
        targets: { nb: [{ text: "Nullstilles " }, { ph: { id: "1", equiv: "date" } }] },
      }),
    ]);
    expect(dict.kept).toBe("Nullstilles {date}");
  });

  it("leaves out one that dropped a placeholder", async () => {
    const dict = await compile([
      block({
        hash: "dropped",
        source: [{ text: "Reset on " }, { ph: { id: "1", equiv: "date" } }],
        targets: { nb: [{ text: "Nullstilles på nytt" }] },
      }),
    ]);
    expect(dict).not.toHaveProperty("dropped");
  });

  it("leaves out one that dropped a paired code", async () => {
    // The markers are what tx() re-attaches the element to; without them the
    // sentence renders as text and the link goes nowhere.
    const dict = await compile([
      block({
        hash: "unpaired",
        source: [
          { pcOpen: { id: "0", type: "jsx:element" } },
          { text: "docs" },
          { pcClose: { id: "0", type: "jsx:element" } },
        ],
        targets: { nb: [{ text: "dokumentasjon" }] },
      }),
    ]);
    expect(dict).not.toHaveProperty("unpaired");
  });

  it("leaves out one that invented a placeholder the source never had", async () => {
    // An extra token is not a missing value, it is a literal brace shown to a
    // reader — a translator's typo arriving in production dressed as markup.
    const dict = await compile([
      block({
        hash: "invented",
        source: [{ text: "Welcome" }],
        targets: { nb: [{ text: "Velkommen " }, { ph: { id: "1", equiv: "name" } }] },
      }),
    ]);
    expect(dict).not.toHaveProperty("invented");
  });

  it("counts them, so a target reusing one token where the source had two does not ship", async () => {
    const dict = await compile([
      block({
        hash: "halved",
        source: [
          { ph: { id: "1", equiv: "from" } },
          { text: " to " },
          { ph: { id: "2", equiv: "to" } },
        ],
        targets: { nb: [{ ph: { id: "1", equiv: "from" } }, { text: " til " }] },
      }),
    ]);
    expect(dict).not.toHaveProperty("halved");
  });

  it("still ships everything sound in the same file", async () => {
    const dict = await compile([
      block({
        hash: "sound",
        source: [{ text: "Save" }],
        targets: { nb: [{ text: "Lagre" }] },
      }),
      block({
        hash: "unsound",
        source: [{ text: "Reset on " }, { ph: { id: "1", equiv: "date" } }],
        targets: { nb: [{ text: "Nullstilles" }] },
      }),
    ]);
    expect(dict).toEqual({ sound: "Lagre" });
  });
});
