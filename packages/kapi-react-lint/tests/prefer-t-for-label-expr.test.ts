import { describe, it } from "vitest";
import { run } from "./rule-tester.ts";
import { rule } from "../src/rules/prefer-t-for-label-expr.ts";

describe("prefer-t-for-label-expr", () => {
  it("valid + invalid cases", () => {
    run("prefer-t-for-label-expr", rule, {
      valid: [
        // JSXText is extractable — no flag.
        { code: `const el = <div>Hello</div>;` },
        // Bare identifier — not a member access. `{count}` could be
        // many things; rule is only confident about label-like props.
        { code: `const el = <div>{count}</div>;` },
        // Member access whose property isn't label-like.
        { code: `const el = <div>{items.length}</div>;` },
        { code: `const el = <div>{user.id}</div>;` },
        // Computed property — can't know the name statically.
        { code: `const el = <div>{obj[key]}</div>;` },
        // Attribute position — out of scope (covered by translatable-attr rules).
        { code: `<img alt={meta.label} />` },
        // Already wrapped with t().
        { code: `const el = <div>{t(meta.label)}</div>;` },
        // translate="no" on the enclosing element suppresses the check.
        { code: `const el = <div translate="no">{meta.label}</div>;` },
        // translate="no" on an ancestor also suppresses.
        {
          code: `const el = <section translate="no"><div><span>{meta.label}</span></div></section>;`,
        },
      ],
      invalid: [
        {
          code: `const el = <div>{meta.label}</div>;`,
          errors: [{ messageId: "dynLabel", data: { expr: "meta.label", key: "label" } }],
        },
        {
          code: `const el = <h1>{item.title}</h1>;`,
          errors: [{ messageId: "dynLabel", data: { expr: "item.title", key: "title" } }],
        },
        {
          code: `const el = <p>{entry.caption}</p>;`,
          errors: [{ messageId: "dynLabel", data: { expr: "entry.caption", key: "caption" } }],
        },
        // Nested member — display uses the nearest identifier.
        {
          code: `const el = <span>{x.obj.tooltip}</span>;`,
          errors: [{ messageId: "dynLabel", data: { expr: "obj.tooltip", key: "tooltip" } }],
        },
        // Custom keys via options.
        {
          code: `const el = <div>{item.cta}</div>;`,
          options: [{ keys: ["cta"] }],
          errors: [{ messageId: "dynLabel", data: { expr: "item.cta", key: "cta" } }],
        },
      ],
    });
  });
});
