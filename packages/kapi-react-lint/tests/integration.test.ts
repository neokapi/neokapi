import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { Linter } from "eslint";
import type { ESLint } from "eslint";
import { describe, expect, it } from "vitest";
import { plugin } from "../src/index.ts";
import { recommendedStrict } from "../src/configs/recommended-strict.ts";

/**
 * Integration check: run the full recommended-strict config against
 * the common-mistakes.tsx fixture and assert each rule fires exactly
 * where we expect. This is what catches regressions where a rule
 * silently stops working (import path change, plugin name mismatch,
 * rule selector typo, etc.).
 */

const here = dirname(fileURLToPath(import.meta.url));
const fixturePath = resolve(here, "..", "examples", "common-mistakes.jsx");

describe("integration: recommended-strict fires on every mistake", () => {
  it("flags one issue per rule in the fixture", () => {
    const linter = new Linter({ configType: "flat" });
    const code = readFileSync(fixturePath, "utf8");
    const messages = linter.verify(code, [
      {
        languageOptions: {
          ecmaVersion: 2023,
          sourceType: "module",
          parserOptions: { ecmaFeatures: { jsx: true } },
        },
        // Rules are authored against @oxlint/plugins types; bridge to
        // ESLint's plugin type at this ESLint-path test boundary.
        plugins: { "kapi-react": plugin as unknown as ESLint.Plugin },
        rules: recommendedStrict.rules,
      },
    ]);

    const ruleIds = messages
      .map((m) => m.ruleId)
      .filter(Boolean)
      .sort();
    const expected = [
      "kapi-react/no-concat-in-translatable-attr",
      "kapi-react/no-concat-in-translatable-attr",
      "kapi-react/no-string-literal-jsx-expr",
      "kapi-react/prefer-t-for-label-props",
      "kapi-react/prefer-t-for-label-props",
      "kapi-react/t-literal-first-arg",
      "kapi-react/t-no-concat",
      "kapi-react/t-no-concat",
    ].sort();
    expect(ruleIds).toEqual(expected);

    // Every finding must be an `error` under strict preset.
    for (const m of messages) {
      expect(m.severity).toBe(2);
    }
  });
});
