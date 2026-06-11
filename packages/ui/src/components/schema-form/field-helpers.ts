import type { PropertySchema } from "./types";

/**
 * Coerces the raw string from a numeric `<input>` to the schema's number
 * type. An empty input clears the value (undefined) instead of producing NaN.
 */
export function coerceNumericInput(raw: string, type: PropertySchema["type"]): number | undefined {
  if (raw === "") return undefined;
  return type === "integer" ? parseInt(raw) : parseFloat(raw);
}

/**
 * Resolves the display label for an enum value: labeled `options` win,
 * then the deprecated `ui:enum-labels` map, then the raw value.
 */
export function enumOptionLabel(
  val: unknown,
  options: PropertySchema["options"],
  enumLabels: Record<string, string> | undefined,
): string {
  if (options) {
    const opt = options.find((o) => String(o.value) === String(val));
    return opt?.label ?? String(val);
  }
  return enumLabels?.[String(val)] ?? String(val);
}

/** Splits the comma-separated string value model of the `tags` widget into tags. */
export function splitTagList(raw: string): string[] {
  if (!raw) return [];
  return raw
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
}
