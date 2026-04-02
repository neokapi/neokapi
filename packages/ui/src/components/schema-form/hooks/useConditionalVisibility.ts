import type { ConditionExpr, PropertySchema } from "../types";

export function evaluateCondition(
  condition: ConditionExpr | undefined,
  values?: Record<string, unknown>,
  properties?: Record<string, PropertySchema>,
): boolean {
  if (!condition) return true;
  if (!values) return true;

  if ("all" in condition) return condition.all.every((c) => evaluateCondition(c, values, properties));
  if ("any" in condition) return condition.any.some((c) => evaluateCondition(c, values, properties));
  if ("not" in condition) return !evaluateCondition(condition.not, values, properties);

  if ("empty" in condition) {
    const v = values[condition.field];
    const isEmpty = v === undefined || v === null || v === "";
    return condition.empty ? isEmpty : !isEmpty;
  }

  if ("eq" in condition) {
    const v = values[condition.field] ?? properties?.[condition.field]?.default;
    return v === condition.eq || String(v) === String(condition.eq);
  }

  return true;
}
