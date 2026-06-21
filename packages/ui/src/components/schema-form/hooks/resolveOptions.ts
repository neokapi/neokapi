import type { OptionItem, PropertySchema } from "../types";
import { evaluateCondition } from "./useConditionalVisibility";

/**
 * resolveEffectiveOptions computes a select field's options, honoring a
 * cascading `ui:option-sets` declaration: when present, the options are those of
 * every set whose `when` condition holds against the current values, falling
 * back to the flat `options` union when no set matches (e.g. the gating field is
 * unset). Without option-sets it is just the field's `options`.
 */
export function resolveEffectiveOptions(
  schema: PropertySchema,
  allValues?: Record<string, unknown>,
  allProperties?: Record<string, PropertySchema>,
): OptionItem[] | undefined {
  const sets = schema["ui:option-sets"];
  if (sets && sets.length > 0) {
    const matched = sets
      .filter((set) => evaluateCondition(set.when, allValues, allProperties))
      .flatMap((set) => set.options);
    if (matched.length > 0) return matched;
  }
  return schema.options;
}
