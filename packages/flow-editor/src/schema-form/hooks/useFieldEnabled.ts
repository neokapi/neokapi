import { evaluateCondition } from "./useConditionalVisibility";
import type { PropertySchema, ConditionExpr } from "../types";

export function useFieldEnabled(
  enabledExpr: ConditionExpr | undefined,
  allValues: Record<string, unknown> | undefined,
  allProperties: Record<string, PropertySchema> | undefined,
): boolean {
  if (!enabledExpr) return true;
  return evaluateCondition(enabledExpr, allValues, allProperties);
}
