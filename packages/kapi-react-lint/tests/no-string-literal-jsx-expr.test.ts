import { describe, it } from "vitest";
import { run } from "./rule-tester.ts";
import { rule } from "../src/rules/no-string-literal-jsx-expr.ts";

describe("no-string-literal-jsx-expr", () => {
  it("valid + invalid cases", () => {
    run("no-string-literal-jsx-expr", rule, {
      valid: [
        { code: `const x = <p>Hello</p>;` },
        { code: `const x = <p>{name}</p>;` },
        // Non-string literal inside expression — unrelated.
        { code: `const x = <p>{42}</p>;` },
        // String literal inside attribute — unrelated, handled elsewhere.
        { code: `const x = <p title={'hi'}>x</p>;` },
        // Whitespace-only literals are fine (someone collapsing JSX).
        { code: `const x = <p>{' '}</p>;` },
      ],
      invalid: [
        {
          code: `const x = <p>{'Hello'}</p>;`,
          output: `const x = <p>Hello</p>;`,
          errors: [{ messageId: "bareLiteral" }],
        },
        {
          code: `const x = <div>{"Save"}</div>;`,
          output: `const x = <div>Save</div>;`,
          errors: [{ messageId: "bareLiteral" }],
        },
      ],
    });
  });
});
