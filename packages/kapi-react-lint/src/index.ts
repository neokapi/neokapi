import type { Rule } from "@oxlint/plugins";
import { rule as tLiteralFirstArg } from "./rules/t-literal-first-arg.ts";
import { rule as tNoConcat } from "./rules/t-no-concat.ts";
import { rule as noConcatInTranslatableAttr } from "./rules/no-concat-in-translatable-attr.ts";
import { rule as noStringLiteralJsxExpr } from "./rules/no-string-literal-jsx-expr.ts";
import { rule as preferTForLabelProps } from "./rules/prefer-t-for-label-props.ts";
import { rule as preferTForLabelExpr } from "./rules/prefer-t-for-label-expr.ts";
import { rule as noTernaryInTranslatableAttr } from "./rules/no-ternary-in-translatable-attr.ts";
import { rule as noTernaryLiteralsInJsxChild } from "./rules/no-ternary-literals-in-jsx-child.ts";

/**
 * The plugin object. The exact same object works for both ESLint
 * flat-config and oxlint's `jsPlugins` — oxlint's plugin API is a
 * strict subset of ESLint v9's, so no adapter layer is needed.
 *
 * Consumers generally want the shareable configs from ./configs/*
 * rather than picking rules individually.
 */
export const plugin = {
  meta: {
    name: "kapi-react",
    version: "0.1.0",
  },
  rules: {
    "t-literal-first-arg": tLiteralFirstArg,
    "t-no-concat": tNoConcat,
    "no-concat-in-translatable-attr": noConcatInTranslatableAttr,
    "no-string-literal-jsx-expr": noStringLiteralJsxExpr,
    "no-ternary-in-translatable-attr": noTernaryInTranslatableAttr,
    "no-ternary-literals-in-jsx-child": noTernaryLiteralsInJsxChild,
    "prefer-t-for-label-props": preferTForLabelProps,
    "prefer-t-for-label-expr": preferTForLabelExpr,
  } satisfies Record<string, Rule>,
} as const;

export default plugin;
