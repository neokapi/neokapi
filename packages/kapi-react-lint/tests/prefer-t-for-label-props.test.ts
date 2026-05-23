import { describe, it } from "vitest";
import { run } from "./rule-tester.ts";
import { rule } from "../src/rules/prefer-t-for-label-props.ts";

describe("prefer-t-for-label-props", () => {
  it("valid + invalid cases", () => {
    run("prefer-t-for-label-props", rule, {
      valid: [
        // Non-label keys — ignored even with string literals.
        { code: `const X = [{ id: 'light', value: 'light' }];` },
        // Call expression values — already computed, nothing to suggest.
        {
          code: `import { t } from '@neokapi/kapi-react/runtime';\n const X = [{ label: t('Light') }];`,
        },
        // Dynamic key.
        { code: `const X = { [key]: 'Light' };` },
        // Shorthand — no literal to flag.
        { code: `const label = 'Light'; const X = { label };` },
        // Non-string value — variable reference etc.
        { code: `const X = { label: ref };` },
      ],
      invalid: [
        {
          code: `const THEMES = [{ label: 'Light' }];`,
          errors: [{ messageId: "useT", data: { key: "label" } }],
        },
        {
          // `description` was dropped from the default key list (too
          // noisy on backend data) — only `title` fires here now.
          code: `const X = { title: 'Welcome', description: 'Get started' };`,
          errors: [{ messageId: "useT", data: { key: "title" } }],
        },
        // Custom keys via options.
        {
          code: `const X = { cta: 'Sign up' };`,
          options: [{ keys: ["cta"] }],
          errors: [{ messageId: "useT", data: { key: "cta" } }],
        },
      ],
    });
  });
});
