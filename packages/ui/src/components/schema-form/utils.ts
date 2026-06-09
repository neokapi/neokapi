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

/**
 * Turn a property key into a human-readable label as a fallback when the schema
 * has no `title`: split camelCase / snake_case / kebab-case into words and
 * title-case them, so `checkLeadingWhitespace` reads "Check Leading Whitespace"
 * and `target_language` reads "Target Language". An explicit `title` always wins.
 */
export function humanizeKey(key: string): string {
  const words = key
    .replace(/[_-]+/g, " ")
    .replace(/([a-z\d])([A-Z])/g, "$1 $2")
    .replace(/([A-Z]+)([A-Z][a-z])/g, "$1 $2")
    .trim();
  if (!words) return key;
  return words
    .split(/\s+/)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}
