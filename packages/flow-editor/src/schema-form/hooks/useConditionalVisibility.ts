import type { ConditionExpr, PropertySchema } from "../types";

export function evaluateCondition(
  condition: ConditionExpr | undefined,
  allValues: Record<string, unknown> | undefined,
  allProperties: Record<string, PropertySchema> | undefined,
): boolean {
  if (!condition || !allValues) return true;

  if ("all" in condition) return condition.all.every(c => evaluateCondition(c, allValues, allProperties));
  if ("any" in condition) return condition.any.some(c => evaluateCondition(c, allValues, allProperties));
  if ("not" in condition) return !evaluateCondition(condition.not, allValues, allProperties);

  const rawVal = allValues[condition.field];
  const defaultVal = allProperties?.[condition.field]?.default;
  const fieldVal = rawVal ?? defaultVal;

  if ("empty" in condition) {
    const isEmpty = fieldVal === undefined || fieldVal === null || fieldVal === "";
    return condition.empty ? isEmpty : !isEmpty;
  }

  return String(fieldVal ?? "") === String(condition.eq);
}
