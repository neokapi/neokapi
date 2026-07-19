import { mkdtempSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, it } from "vitest";
import type { File } from "@neokapi/kapi-format";
import { newFile, marshalFile } from "@neokapi/kapi-format";

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
            source: [
              { text: "Welcome, " },
              { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
              { text: "!" },
            ],
            targets: {
              qps: [
                { text: "[Ŵéļçöḿé, " },
                { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
                { text: "!]" },
              ],
              de: [
                { text: "Willkommen, " },
                { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
                { text: "!" },
              ],
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
    const klfDir = join(dir, "i18n");
    const outDir = join(dir, "out");
    mkdirSync(klfDir, { recursive: true });
    writeFileSync(join(klfDir, "App.klf"), marshalFile(translatedFile()));

    await runCompile([klfDir, "--out", outDir]);

    const written = readdirSync(outDir).sort();
    expect(written).toEqual(["de.json", "qps.json"]);

    const qps = JSON.parse(readFileSync(join(outDir, "qps.json"), "utf-8"));
    expect(qps).toEqual({ "h-welcome": "[Ŵéļçöḿé, {name}!]" });

    const de = JSON.parse(readFileSync(join(outDir, "de.json"), "utf-8"));
    expect(de).toEqual({ "h-welcome": "Willkommen, {name}!" });
  });

  it("honors --locale filter", async () => {
    const dir = tempDir("compile");
    const klfDir = join(dir, "i18n");
    const outDir = join(dir, "out");
    mkdirSync(klfDir, { recursive: true });
    writeFileSync(join(klfDir, "App.klf"), marshalFile(translatedFile()));

    await runCompile([klfDir, "--locale", "qps", "--out", outDir]);

    const written = readdirSync(outDir);
    expect(written).toEqual(["qps.json"]);
  });

  it("compiles from a single .klf file", async () => {
    const dir = tempDir("compile-single");
    const klfPath = join(dir, "App.klf");
    const outDir = join(dir, "out");
    writeFileSync(klfPath, marshalFile(translatedFile()));

    await runCompile([klfPath, "--out", outDir]);

    const written = readdirSync(outDir).sort();
    expect(written).toEqual(["de.json", "qps.json"]);
    const qps = JSON.parse(readFileSync(join(outDir, "qps.json"), "utf-8"));
    expect(qps["h-welcome"]).toBeTruthy();
  });
});
