import { describe, it } from "vitest";
import { run } from "./rule-tester.ts";
import { rule } from "../src/rules/t-no-concat.ts";

const IMPORT = `import { t } from '@neokapi/kapi-react/runtime';\n`;

describe("t-no-concat", () => {
  it("valid + invalid cases", () => {
    run("t-no-concat", rule, {
      valid: [
        { code: `${IMPORT}t('Hello {name}');` },
        // Template with no expressions — fine.
        { code: `${IMPORT}t(\`Hello world\`);` },
        // Non-string `+` still counts as stringish only if one side
        // is stringish; `1 + 2` should not trigger.
        { code: `${IMPORT}t(1 + 2);` },
        // Unknown `t`.
        { code: `t('A ' + b);` },
      ],
      invalid: [
        {
          code: `${IMPORT}t('Hello ' + name);`,
          errors: [{ messageId: "concat" }],
        },
        {
          code: `${IMPORT}t(\`Hello \${name}\`);`,
          errors: [{ messageId: "template" }],
        },
        {
          code: `${IMPORT}t(prefix + ' world');`,
          errors: [{ messageId: "concat" }],
        },
      ],
    });
  });
});
