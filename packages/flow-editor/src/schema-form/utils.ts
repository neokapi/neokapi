import { theme } from "../theme";
import type { PropertySchema } from "./types";

export { theme };

export function inputStyle(compact: boolean): React.CSSProperties {
  return {
    width: "100%",
    padding: compact ? "3px 6px" : "5px 8px",
    fontSize: compact ? 11 : 12,
    borderRadius: 4,
    border: `1px solid ${theme.border}`,
    background: theme.bgCard,
    color: theme.fg,
    fontFamily: "inherit",
    outline: "none",
    boxSizing: "border-box",
  };
}

export const removeButtonStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: 18,
  height: 18,
  borderRadius: 4,
  border: "none",
  background: "transparent",
  cursor: "pointer",
  color: theme.fgMuted,
  flexShrink: 0,
};

export function formatLabel(name: string): string {
  return name
    .replace(/([A-Z])/g, " $1")
    .replace(/^./, (s) => s.toUpperCase())
    .trim();
}

/**
 * Resolve $ref-style additionalProperties. In our schemas, refs point
 * to $defs which aren't resolved at runtime, so we return the raw
 * additionalProperties schema if it's an object, otherwise undefined.
 */
export function resolveRef(schema: PropertySchema): PropertySchema | undefined {
  const ap = schema.additionalProperties;
  if (ap && typeof ap === "object") {
    return ap;
  }
  return undefined;
}

export function hasAdditionalProperties(schema: PropertySchema): boolean {
  return schema.additionalProperties != null && schema.additionalProperties !== false;
}
