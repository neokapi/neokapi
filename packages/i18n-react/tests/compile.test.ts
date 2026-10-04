import { mkdtempSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, it } from "vitest";
import type { BlockV1, File } from "@neokapi/kapi-format";
import { KindI18nReact, SchemaVersionV1, newFile, marshalFile } from "@neokapi/kapi-format";

import { runCompile } from "../src/commands/compile.ts";

function tempDir(prefix: string) {
  return mkdtempSync(join(tmpdir(), `${prefix}-`));
}

// A minimal block with both source + targets populated so compile
// has real content to flatten.
function translatedFile(): File {
  return newFile({
    generator: { id: "test", version: "1" },
    project: { id: "compile-test", sourceLocale: "en" },
    documents: [
      {
        id: "App",
        documentType: "jsx",
        path: "App.tsx",
        blocks: [
          {
            id: "welcome",
            hash: "h-welcome",
            translatable: true,
            type: "jsx:element",
            editions: {
              "": {
                runs: [
                  { text: "Welcome, " },
                  { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
                  { text: "!" },
                ],
              },
              qps: {
                runs: [
                  { text: "[Ŵéļçöḿé, " },
                  { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
                  { text: "!]" },
                ],
              },
              de: {
                runs: [
                  { text: "Willkommen, " },
                  { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
                  { text: "!" },
                ],
                status: "translated",
                origin: { kind: "ai" },
              },
              // A tone edition is not a language a runtime looks a catalog up by.
              "de;tone=formal": {
                runs: [
                  { text: "Willkommen, " },
                  { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
                  { text: "! (Sie)" },
                ],
              },
            },
            placeholders: [{ name: "name", kind: "variable", sourceExpr: "name" }],
            properties: {
              file: "App.tsx",
              line: 1,
              component: "App",
              jsxPath: "h1",
              element: "h1",
            },
          },
        ],
      },
    ],
  });
}

describe("runCompile", () => {
  it("emits one JSON per locale with hash→flattened-text entries", async () => {
    const dir = tempDir("compile");
    const kbfDir = join(dir, "i18n");
    const outDir = join(dir, "out");
    mkdirSync(kbfDir, { recursive: true });
    writeFileSync(join(kbfDir, "App.kbf.json"), marshalFile(translatedFile()));

    await runCompile([kbfDir, "--out", outDir]);

    const written = readdirSync(outDir).sort();
    expect(written).toEqual(["de.json", "qps.json"]);

    const qps = JSON.parse(readFileSync(join(outDir, "qps.json"), "utf-8"));
    expect(qps).toEqual({ "h-welcome": "[Ŵéļçöḿé, {name}!]" });

    const de = JSON.parse(readFileSync(join(outDir, "de.json"), "utf-8"));
    expect(de).toEqual({ "h-welcome": "Willkommen, {name}!" });
  });

  it("honors --locale filter", async () => {
    const dir = tempDir("compile");
    const kbfDir = join(dir, "i18n");
    const outDir = join(dir, "out");
    mkdirSync(kbfDir, { recursive: true });
    writeFileSync(join(kbfDir, "App.kbf.json"), marshalFile(translatedFile()));

    await runCompile([kbfDir, "--locale", "qps", "--out", outDir]);

    const written = readdirSync(outDir);
    expect(written).toEqual(["qps.json"]);
  });

  it("compiles a catalog an earlier kapi or this package's 1.2.3 release wrote in schema 1.0", async () => {
    const dir = tempDir("compile-schema1");
    const kbfPath = join(dir, "App.klf");
    const outDir = join(dir, "out");
    const block: BlockV1 = {
      id: "welcome",
      hash: "h-welcome",
      translatable: true,
      type: "jsx:element",
      source: [
        { text: "Welcome, " },
        { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
      ],
      targets: {
        de: [
          { text: "Willkommen, " },
          { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
        ],
      },
      placeholders: [{ name: "name", kind: "variable", sourceExpr: "name" }],
      properties: { file: "App.tsx", line: 1, component: "App", jsxPath: "h1", element: "h1" },
    };
    writeFileSync(
      kbfPath,
      JSON.stringify({
        schemaVersion: SchemaVersionV1,
        kind: KindI18nReact,
        generator: { id: "test", version: "1" },
        project: { id: "compile-test", sourceLocale: "en" },
        documents: [{ id: "App", documentType: "jsx", path: "App.tsx", blocks: [block] }],
      }),
    );

    await runCompile([kbfPath, "--out", outDir]);

    expect(readdirSync(outDir)).toEqual(["de.json"]);
    expect(JSON.parse(readFileSync(join(outDir, "de.json"), "utf-8"))).toEqual({
      "h-welcome": "Willkommen, {name}",
    });
  });

  it("refuses a catalog in a schema it does not read", async () => {
    const dir = tempDir("compile-schema3");
    const kbfPath = join(dir, "App.kbf.json");
    writeFileSync(
      kbfPath,
      JSON.stringify({ schemaVersion: "3.0", kind: "kapi-bundle", documents: [] }),
    );
    await expect(runCompile([kbfPath, "--out", join(dir, "out")])).rejects.toThrow(
      /unsupported major schemaVersion 3/,
    );
  });

  it("compiles from a single .kbf.json file", async () => {
    const dir = tempDir("compile-single");
    const kbfPath = join(dir, "App.kbf.json");
    const outDir = join(dir, "out");
    writeFileSync(kbfPath, marshalFile(translatedFile()));

    await runCompile([kbfPath, "--out", outDir]);

    const written = readdirSync(outDir).sort();
    expect(written).toEqual(["de.json", "qps.json"]);
    const qps = JSON.parse(readFileSync(join(outDir, "qps.json"), "utf-8"));
    expect(qps["h-welcome"]).toBeTruthy();
  });
});
