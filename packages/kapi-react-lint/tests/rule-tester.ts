import { RuleTester } from "eslint";

/**
 * Shared RuleTester for all rules in this package. Uses the ESLint 9
 * flat-config language options — enables JSX and modern ES syntax so
 * tests don't each need to repeat the parser configuration.
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
