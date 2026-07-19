import { RuleTester } from "eslint";
import type { Rule as ESLintRule } from "eslint";
import type { Rule as OxlintRule } from "@oxlint/plugins";

/**
 * Shared RuleTester for all rules in this package. Uses the ESLint 9
 * flat-config language options — enables JSX and modern ES syntax so
 * tests don't each need to repeat the parser configuration.
 *
 * This harness verifies the **ESLint** consumption path. Rules are
 * authored against `@oxlint/plugins` types (oxlint's plugin API is a
 * strict subset of ESLint v9's), so the rule objects are runtime
 * compatible with both linters; we only bridge the static types here.
 * The oxlint consumption path is covered by `oxlint.test.ts`.
 */
export const ruleTester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2023,
    sourceType: "module",
    parserOptions: {
      ecmaFeatures: { jsx: true },
    },
  },
});

/**
 * Run an oxlint-authored rule through ESLint's RuleTester. Bridges the
 * `@oxlint/plugins` rule type to ESLint's `RuleModule` at this boundary
 * (see note above) so test files don't each repeat the cast.
 */
export function run(name: string, rule: OxlintRule, cases: Parameters<RuleTester["run"]>[2]): void {
  ruleTester.run(name, rule as unknown as ESLintRule.RuleModule, cases);
}
