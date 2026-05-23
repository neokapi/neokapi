import { describe, it } from "vitest";
import { run } from "./rule-tester.ts";
import { rule } from "../src/rules/no-ternary-literals-in-jsx-child.ts";

describe("no-ternary-literals-in-jsx-child", () => {
  it("valid + invalid cases", () => {
    run("no-ternary-literals-in-jsx-child", rule, {
      valid: [
        // Not a ternary at all.
        { code: `<p>Hello</p>` },
        { code: `<p>{count}</p>` },
        // Ternary with both branches non-string (both `t()` calls,
        // elements, computed values) — assumed intentional.
        { code: `<p>{cond ? t("A") : t("B")}</p>` },
        { code: `<p>{cond ? <A/> : <B/>}</p>` },
        { code: `<p>{cond ? fn() : gn()}</p>` },
        { code: `<p>{cond ? value : otherValue}</p>` },
        // translate="no" on the element suppresses.
        { code: `<p translate="no">{cond ? "A" : "B"}</p>` },
        // translate="no" on an ancestor suppresses.
        { code: `<div translate="no"><p>{cond ? "A" : "B"}</p></div>` },
        // Attribute position — out of scope (covered by no-ternary-in-translatable-attr).
        { code: `<input placeholder={cond ? "A" : "B"} />` },
        // Format-only templates (no alphabetic text) aren't translatable copy.
        { code: "<span>{cond ? `${pct}%` : t('Loading...')}</span>" },
        { code: "<span>{cond ? `v${version}` : t('Update')}</span>" },
      ],
      invalid: [
        {
          code: `<p>{cond ? "A" : "B"}</p>`,
          errors: [{ messageId: "literalBranch" }],
        },
        {
          code: `<Button>{loading ? "Saving..." : "Save"}</Button>`,
          errors: [{ messageId: "literalBranch" }],
        },
        // Only one branch is a literal — still flag (that branch is lost).
        {
          code: `<Button>{loading ? t("Saving...") : "Save"}</Button>`,
          errors: [{ messageId: "literalBranch" }],
        },
        // Template literal branches also flagged — same extractor behaviour.
        {
          code: '<p>{cond ? `Loading ${n}...` : "Done"}</p>',
          errors: [{ messageId: "literalBranch" }],
        },
        // Inside a React Fragment.
        {
          code: `<>{cond ? "A" : "B"}</>`,
          errors: [{ messageId: "literalBranch" }],
        },
      ],
    });
  });
});
