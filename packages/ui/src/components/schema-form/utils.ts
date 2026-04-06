import type { PropertySchema } from "./types";

/** Resolve additionalProperties to a concrete PropertySchema. */
export function resolveRef(schema: PropertySchema): PropertySchema | undefined {
  if (schema.additionalProperties && typeof schema.additionalProperties === "object") {
    return schema.additionalProperties;
  }
  return undefined;
}

/** Check if schema declares additionalProperties. */
export function hasAdditionalProperties(schema: PropertySchema): boolean {
  return schema.additionalProperties !== undefined && schema.additionalProperties !== false;
}
