import { execFileSync } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";
import { beforeAll, describe, expect, it } from "vitest";

/**
 * Integration check for the **oxlint** consumption path — the way the
 * plugin is actually loaded in production (`jsPlugins` in .oxlintrc.json,
 * see apps/kapi-desktop/frontend). The sibling integration.test.ts covers
 * the ESLint path; both must flag the exact same issues on the same
 * fixture, proving the single rule source is genuinely dual-linter.
 *
 * We bundle src/oxlint.ts to a temp ESM file and point oxlint at it via
 * an absolute jsPlugins path, so the test exercises the real rule source
 * with no dependency on a prior `npm run build`.
 */

const here = dirname(fileURLToPath(import.meta.url));
const pkgRoot = resolve(here, "..");
const repoRoot = resolve(pkgRoot, "..", "..");
const oxlintBin = join(repoRoot, "node_modules", ".bin", "oxlint");
const fixture = resolve(pkgRoot, "examples", "common-mistakes.jsx");

let pluginPath = "";

beforeAll(async () => {
  const dir = mkdtempSync(join(tmpdir(), "krl-oxlint-"));
  pluginPath = join(dir, "plugin.mjs");
  await build({
    entryPoints: [resolve(pkgRoot, "src", "oxlint.ts")],
    outfile: pluginPath,
    bundle: true,
    format: "esm",
    platform: "node",
    target: "node22",
  });
}, 30_000);

describe("integration: oxlint consumption path", () => {
  it("flags one issue per rule in the fixture (matching the ESLint path)", () => {
    const dir = mkdtempSync(join(tmpdir(), "krl-oxcfg-"));
    const configPath = join(dir, ".oxlintrc.json");
    writeFileSync(
      configPath,
      JSON.stringify({
        jsPlugins: [pluginPath],
        rules: {
          "kapi-react/t-literal-first-arg": "error",
          "kapi-react/t-no-concat": "error",
          "kapi-react/no-concat-in-translatable-attr": "error",
          "kapi-react/no-ternary-in-translatable-attr": "error",
          "kapi-react/no-ternary-literals-in-jsx-child": "error",
          "kapi-react/no-string-literal-jsx-expr": "warn",
          "kapi-react/prefer-t-for-label-props": "warn",
          "kapi-react/prefer-t-for-label-expr": "warn",
        },
      }),
    );

    // oxlint exits non-zero when it reports problems; the JSON report is
    // still written to stdout, so read it off the thrown error.
    let stdout: string;
    try {
      stdout = execFileSync(oxlintBin, ["-c", configPath, "--format=json", fixture], {
        encoding: "utf8",
      });
    } catch (err) {
      stdout = (err as { stdout?: string }).stdout ?? "";
    }

    const report = JSON.parse(stdout) as {
      diagnostics: { code: string }[];
    };
    const ruleIds = report.diagnostics
      .map((d) => /kapi-react\((.+?)\)/.exec(d.code)?.[1])
      .filter((id): id is string => Boolean(id))
      .sort();

    // Identical expectation to integration.test.ts (the ESLint path).
    const expected = [
      "no-concat-in-translatable-attr",
      "no-concat-in-translatable-attr",
      "no-string-literal-jsx-expr",
      "prefer-t-for-label-props",
      "prefer-t-for-label-props",
      "t-literal-first-arg",
      "t-no-concat",
      "t-no-concat",
    ].sort();

    expect(ruleIds).toEqual(expected);
  });
});
