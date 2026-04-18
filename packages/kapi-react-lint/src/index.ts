import type { Rule } from "eslint";
import { rule as tLiteralFirstArg } from "./rules/t-literal-first-arg.ts";
import { rule as tNoConcat } from "./rules/t-no-concat.ts";
import { rule as noConcatInTranslatableAttr } from "./rules/no-concat-in-translatable-attr.ts";
import { rule as noStringLiteralJsxExpr } from "./rules/no-string-literal-jsx-expr.ts";
import { rule as preferTForLabelProps } from "./rules/prefer-t-for-label-props.ts";

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
    "prefer-t-for-label-props": preferTForLabelProps,
  } satisfies Record<string, Rule.RuleModule>,
} as const;

export default plugin;
