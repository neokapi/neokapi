/**
 * `--warnings-json` records every warning a run raised, sorted so two runs over
 * one tree write the same bytes. The repository's extractor ratchet compares
 * the file with a committed baseline.
 */

import { describe, expect, it, vi } from "vitest";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { runExtract } from "../src/commands/extract.ts";

/** A package with the given source files, extracted with `args`; returns the root. */
async function extractIn(sources: Record<string, string>, args: string[]): Promise<string> {
  const root = mkdtempSync(join(tmpdir(), "i18n-react-warnings-"));
  for (const [file, code] of Object.entries(sources)) {
    mkdirSync(join(root, file, ".."), { recursive: true });
    writeFileSync(join(root, file), code);
  }
  const cwd = process.cwd();
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  process.chdir(root);
  try {
    await runExtract(["--src", "src/**/*.tsx", "--out", "i18n", ...args]);
  } finally {
    process.chdir(cwd);
    log.mockRestore();
    warn.mockRestore();
  }
  return root;
}

describe("extract --warnings-json", () => {
  it("writes each warning's kind, file, line and tag, sorted by file and line", async () => {
    const root = await extractIn(
      {
        "src/b.tsx": "export const B = () => <TabsTrigger>Settings</TabsTrigger>;\n",
        "src/a.tsx": [
          "export const A = ({ on, name }: { on: boolean; name: string }) => (",
          '  <input placeholder={on ? "Search" : name} />',
          ");",
          "",
        ].join("\n"),
      },
      ["--warnings-json", "out/warnings.json"],
    );
    try {
      const written = JSON.parse(readFileSync(join(root, "out", "warnings.json"), "utf8"));
      expect(written).toEqual({
        warnings: [
          { kind: "ternary-attr-complex", file: "src/a.tsx", line: 2, tag: "placeholder" },
          { kind: "unknown-component", file: "src/b.tsx", line: 1, tag: "TabsTrigger" },
        ],
      });
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("writes an empty list for a run that raised no warning", async () => {
    const root = await extractIn({ "src/a.tsx": "export const A = () => <h1>Hello</h1>;\n" }, [
      "--warnings-json",
      "warnings.json",
    ]);
    try {
      expect(JSON.parse(readFileSync(join(root, "warnings.json"), "utf8"))).toEqual({
        warnings: [],
      });
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});
