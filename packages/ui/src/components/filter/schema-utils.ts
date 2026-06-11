import { PropertySchema } from "./types";

// ─── Utilities ──────────────────────────────────────────────

export function resolveAdditionalProperties(schema: PropertySchema): PropertySchema | undefined {
  const ap = schema.additionalProperties;
  if (ap && typeof ap === "object") {
    return ap;
  }
  return undefined;
}

export function hasAdditionalProperties(schema: PropertySchema): boolean {
  return schema.additionalProperties != null && schema.additionalProperties !== false;
}
