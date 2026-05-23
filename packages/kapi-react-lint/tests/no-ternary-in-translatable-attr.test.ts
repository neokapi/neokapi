import { describe, it } from "vitest";
import { run } from "./rule-tester.ts";
import { rule } from "../src/rules/no-ternary-in-translatable-attr.ts";

describe("no-ternary-in-translatable-attr", () => {
  it("valid + invalid cases", () => {
    run("no-ternary-in-translatable-attr", rule, {
      valid: [
        // All-literal branches — extractor handles these.
        { code: `<PageHeader title={cond ? "A" : "B"} />` },
        { code: `<Input placeholder={disabled ? "Off" : "On"} />` },
        // Non-translatable attr — ignored.
        { code: `<div className={cond ? "a" : fn()} />` },
        // Not a ternary.
        { code: `<PageHeader title="Fixed" />` },
        { code: `<PageHeader title={foo} />` },
        // Both branches non-literal — rule assumes intentional computed logic.
        { code: `<PageHeader title={cond ? t("A") : t("B")} />` },
        { code: `<PageHeader title={cond ? fn() : gn()} />` },
        // translate="no" on the same element suppresses the check.
        { code: `<PageHeader translate="no" title={cond ? fn() : "B"} />` },
      ],
      invalid: [
        {
          code: `<PageHeader title={cond ? getLabel() : "Flows"} />`,
          errors: [{ messageId: "mixed", data: { attr: "title" } }],
        },
        {
          code: `<Input placeholder={disabled ? "Off" : getVal()} />`,
          errors: [{ messageId: "mixed", data: { attr: "placeholder" } }],
        },
        {
          code: `<img alt={show ? "Hero" : altText} />`,
          errors: [{ messageId: "mixed", data: { attr: "alt" } }],
        },
        {
          code: `<div aria-label={a ? labelVar : "Open"} />`,
          errors: [{ messageId: "mixed", data: { attr: "aria-label" } }],
        },
      ],
    });
  });
});
