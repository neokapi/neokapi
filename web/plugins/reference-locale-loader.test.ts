import { afterAll, describe, expect, it } from "vite-plus/test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const loader = require("./reference-locale-loader.cjs") as ((source: string) => string) & {
  localeVariantPath: (resourcePath: string, locale: string) => string | null;
};

// The datasets each test writes are removed once the file's tests have run.
const made: string[] = [];
afterAll(() => {
  for (const dir of made.splice(0)) rmSync(dir, { recursive: true, force: true });
});

function referenceData(): string {
  const dir = mkdtempSync(join(tmpdir(), "reference-data-"));
  made.push(dir);
  return dir;
}

function run(resourcePath: string, locale: string, source: string) {
  const deps: string[] = [];
  const out = loader.call(
    { getOptions: () => ({ locale }), resourcePath, addDependency: (p: string) => deps.push(p) },
    source,
  );
  return { out, deps };
}

describe("reference-locale loader", () => {
  it("returns the locale variant beside the dataset and declares it a dependency", () => {
    const data = referenceData();
    mkdirSync(join(data, "qps"));
    writeFileSync(
      join(data, "tools.json"),
      '{"kind":"tool","entries":[{"id":"a","displayName":"Segment"}]}',
    );
    writeFileSync(
      join(data, "qps", "tools.json"),
      '{"kind":"tool","entries":[{"id":"a","displayName":"Šéĝḿéñţ"}]}',
    );

    const { out, deps } = run(join(data, "tools.json"), "qps", "english");

    expect(out).toContain("Šéĝḿéñţ");
    expect(deps).toEqual([join(data, "qps", "tools.json")]);
  });

  it("passes the English through when the locale has no variant", () => {
    const data = referenceData();
    writeFileSync(join(data, "tools.json"), "{}");

    const { out, deps } = run(join(data, "tools.json"), "nb", "english");

    expect(out).toBe("english");
    expect(deps).toEqual([]);
  });

  it("never looks a variant up under itself", () => {
    expect(loader.localeVariantPath("/x/data/qps/tools.json", "qps")).toBeNull();
    expect(loader.localeVariantPath("/x/data/tools.json", "qps")).toBe("/x/data/qps/tools.json");
    const { out } = run("/x/data/qps/tools.json", "qps", "variant");
    expect(out).toBe("variant");
  });
});
