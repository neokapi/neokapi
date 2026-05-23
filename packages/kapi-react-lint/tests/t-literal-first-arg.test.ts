import { describe, it } from "vitest";
import { run } from "./rule-tester.ts";
import { rule } from "../src/rules/t-literal-first-arg.ts";

const IMPORT = `import { t } from '@neokapi/kapi-react/runtime';\n`;

describe("t-literal-first-arg", () => {
  it("valid + invalid cases", () => {
    run("t-literal-first-arg", rule, {
      valid: [
        { code: `${IMPORT}t('Hello');` },
        { code: `${IMPORT}t('Hello', 'UI Language');` },
        { code: `${IMPORT}t(\`Hello world\`);` },
        // No `t` import → rule stays silent.
        { code: `t(variable);` },
        // Different `t` than ours (e.g. i18next).
        { code: `import { t } from 'i18next'; t(variable);` },
        // Aliased import still detected as string literal.
        {
          code: `import { t as tr } from '@neokapi/kapi-react/runtime'; tr('Hi');`,
        },
      ],
      invalid: [
        {
          code: `${IMPORT}t(variable);`,
          errors: [{ messageId: "notLiteral" }],
        },
        {
          code: `${IMPORT}t(getLabel());`,
          errors: [{ messageId: "notLiteral" }],
        },
        {
          code: `${IMPORT}t(cond ? 'A' : 'B');`,
          errors: [{ messageId: "notLiteral" }],
        },
        {
          code: `${IMPORT}t('');`,
          errors: [{ messageId: "emptyString" }],
        },
        {
          code: `import { t as tr } from '@neokapi/kapi-react/runtime';\n` + `tr(x);`,
          errors: [{ messageId: "notLiteral" }],
        },
      ],
    });
  });
});
