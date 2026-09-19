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

  // An ICU plural carries its count inside the picker rather than as a token,
  // so the token comparison sees nothing on either side and lets a flattened
  // target through. The nightly delivered exactly that: the `other` branch on
  // its own, with the picker gone, so a reader is handed a sentence that no
  // longer says how many.
  it("leaves out one that flattened an ICU plural", async () => {
    const dict = await compile([
      block({
        hash: "flattened",
        source: [
          {
            plural: {
              pivot: "judged.length",
              forms: {
                one: [{ text: "The one pair has been judged." }],
                other: [{ text: "Every pair has been judged." }],
              },
            },
          },
        ],
        targets: { nb: [{ text: "Alle par er vurdert." }] },
      }),
    ]);
    expect(dict).not.toHaveProperty("flattened");
  });

  // The half that must not move. How many categories a language needs is a
  // property of that language: English writes two where Japanese writes one.
  // A target writing fewer than its source is correct and still ships.
  it("ships a plural whose target writes fewer categories than the source", async () => {
    const dict = await compile([
      block({
        hash: "fewer",
        source: [
          {
            plural: {
              pivot: "count",
              forms: { one: [{ text: "1 file" }], other: [{ text: "# files" }] },
            },
          },
        ],
        targets: {
          nb: [{ plural: { pivot: "count", forms: { other: [{ text: "# filer" }] } } }],
        },
      }),
    ]);
    expect(dict.fewer).toBe("{count, plural, other {# filer}}");
  });

  // A picker whose argument the target renamed resolves against nothing at
  // render time, so it is as lost as a dropped token.
  it("leaves out one that renamed the picker's argument", async () => {
    const dict = await compile([
      block({
        hash: "renamed",
        source: [
          {
            plural: {
              pivot: "count",
              forms: { one: [{ text: "1 file" }], other: [{ text: "# files" }] },
            },
          },
        ],
        targets: {
          nb: [{ plural: { pivot: "antall", forms: { other: [{ text: "# filer" }] } } }],
        },
      }),
    ]);
    expect(dict).not.toHaveProperty("renamed");
  });

  // A placeholder inside a branch is a hole like any other, and this one is
  // caught today. It stays caught.
  it("leaves out one that dropped a placeholder from inside a branch", async () => {
    const dict = await compile([
      block({
        hash: "branchhole",
        source: [
          {
            plural: {
              pivot: "count",
              forms: {
                one: [{ text: "1 file for " }, { ph: { id: "1", equiv: "name" } }],
                other: [{ text: "# files for " }, { ph: { id: "1", equiv: "name" } }],
              },
            },
          },
        ],
        targets: {
          nb: [{ plural: { pivot: "count", forms: { other: [{ text: "# filer" }] } } }],
        },
      }),
    ]);
    expect(dict).not.toHaveProperty("branchhole");
  });

  // A number, date or time argument is ICU syntax that opens no picker, so
  // there is no head to compare and counting stays the right question. A target
  // that used one token where the source used two has lost a value.
  it("counts tokens in an ICU message that opens no picker", async () => {
    const dict = await compile([
      block({
        hash: "numberarg",
        source: [
          { text: "Updated {n, number} for " },
          { ph: { id: "1", equiv: "name" } },
          { text: " and " },
          { ph: { id: "2", equiv: "name" } },
        ],
        targets: {
          nb: [{ text: "Oppdatert {n, number} for " }, { ph: { id: "1", equiv: "name" } }],
        },
      }),
    ]);
    expect(dict).not.toHaveProperty("numberarg");
  });
});
