import { useMemo } from "react";
import type { ConditionExpr, PropertySchema } from "../types";
import { evaluateCondition } from "./useConditionalVisibility";

export function useFieldEnabled(
  enabledExpr: ConditionExpr | undefined,
  allValues?: Record<string, unknown>,
  allProperties?: Record<string, PropertySchema>,
): boolean {
  return useMemo(
    () => evaluateCondition(enabledExpr, allValues, allProperties),
    [enabledExpr, allValues, allProperties],
  );
}
