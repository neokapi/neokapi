import { describe, it } from "vitest";
import { run } from "./rule-tester.ts";
import { rule } from "../src/rules/no-concat-in-translatable-attr.ts";

describe("no-concat-in-translatable-attr", () => {
  it("valid + invalid cases", () => {
    run("no-concat-in-translatable-attr", rule, {
      valid: [
        // Literal attribute value — fine.
        { code: `const x = <img alt="Logo" />;` },
        // Non-translatable attribute.
        { code: `const x = <div className={'a ' + b} />;` },
        // Expression that's not a concat (e.g. a variable).
        { code: `const x = <img alt={caption} />;` },
      ],
      invalid: [
        {
          code: `const x = <img alt={'Logo ' + brand} />;`,
          errors: [{ messageId: "concat", data: { attr: "alt" } }],
        },
        {
          code: `const x = <button title={\`Save \${fileName}\`} />;`,
          errors: [{ messageId: "template", data: { attr: "title" } }],
        },
        {
          code: `const x = <input placeholder={'Search ' + category} />;`,
          errors: [{ messageId: "concat", data: { attr: "placeholder" } }],
        },
        {
          code: `const x = <X aria-label={'Open ' + name} />;`,
          errors: [{ messageId: "concat", data: { attr: "aria-label" } }],
        },
      ],
    });
  });
});
