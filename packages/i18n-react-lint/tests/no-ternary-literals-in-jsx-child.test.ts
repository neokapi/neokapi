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
        // String-literal branches extract as their own blocks (#2581), so
        // there is nothing to fix and nothing to say.
        { code: `<p>{cond ? "A" : "B"}</p>` },
        { code: `<Button>{loading ? "Saving..." : "Save"}</Button>` },
        { code: `<Button>{loading ? t("Saving...") : "Save"}</Button>` },
        { code: `<>{cond ? "A" : "B"}</>` },
        // translate="no" on the element suppresses.
        { code: '<p translate="no">{cond ? `Loading ${n}...` : "B"}</p>' },
        // translate="no" on an ancestor suppresses.
        { code: '<div translate="no"><p>{cond ? `Loading ${n}...` : "B"}</p></div>' },
        // Attribute position — out of scope (covered by no-ternary-in-translatable-attr).
        { code: '<input placeholder={cond ? `Loading ${n}...` : "B"} />' },
        // Format-only templates (no alphabetic text) aren't translatable copy.
        { code: "<span>{cond ? `${pct}%` : t('Loading...')}</span>" },
        { code: "<span>{cond ? `v${version}` : t('Update')}</span>" },
      ],
      invalid: [
        // A template literal is one opaque expression to the extractor, so its
        // words are lost whichever branch carries it.
        {
          code: '<p>{cond ? `Loading ${n}...` : "Done"}</p>',
          errors: [{ messageId: "literalBranch" }],
        },
        {
          code: '<p>{cond ? "Done" : `Loading ${n}...`}</p>',
          errors: [{ messageId: "literalBranch" }],
        },
        {
          code: "<p>{cond ? `Saving ${n} files` : `Saved ${n} files`}</p>",
          errors: [{ messageId: "literalBranch" }],
        },
        // Inside a React Fragment.
        {
          code: "<>{cond ? `Loading ${n}...` : t('Done')}</>",
          errors: [{ messageId: "literalBranch" }],
        },
      ],
    });
  });
});
