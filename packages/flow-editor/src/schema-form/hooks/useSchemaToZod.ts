/**
 * useSchemaToZod — converts JSON Schema property definitions to a Zod schema
 * for runtime validation via React Hook Form's zodResolver.
 *
 * This is a lightweight conversion that handles the common property types
 * used in neokapi format/tool schemas. It does NOT aim to be a complete
 * JSON Schema to Zod converter — just enough for field-level validation.
 */
import { useMemo } from "react";
import { z, type ZodTypeAny } from "zod";
import type { PropertySchema } from "../types";

/**
 * Convert a map of JSON Schema properties to a Zod object schema.
 * Returns null if no properties or conversion fails.
 */
export function schemaToZod(
  properties: Record<string, PropertySchema> | undefined,
): z.ZodObject<Record<string, ZodTypeAny>> | null {
  if (!properties || Object.keys(properties).length === 0) return null;

  const shape: Record<string, ZodTypeAny> = {};

  for (const [key, prop] of Object.entries(properties)) {
    const field = propertyToZod(prop);
    if (field) {
      shape[key] = field;
    }
  }

  if (Object.keys(shape).length === 0) return null;
  return z.object(shape).passthrough();
}

function propertyToZod(prop: PropertySchema): ZodTypeAny | null {
  let field: ZodTypeAny;

  switch (prop.type) {
    case "string": {
      let s = z.string();
      if (prop.minLength != null) s = s.min(prop.minLength);
      if (prop.maxLength != null) s = s.max(prop.maxLength);
      if (prop.enum && prop.enum.length > 0) {
        const values = prop.enum.map(String);
        field = z.enum(values as [string, ...string[]]);
      } else {
        field = s;
      }
      break;
    }

    case "integer": {
      let n = z.number().int();
      if (prop.minimum != null) n = n.min(prop.minimum);
      if (prop.maximum != null) n = n.max(prop.maximum);
      field = n;
      break;
    }

    case "number": {
      let n = z.number();
      if (prop.minimum != null) n = n.min(prop.minimum);
      if (prop.maximum != null) n = n.max(prop.maximum);
      field = n;
      break;
    }

    case "boolean":
      field = z.boolean();
      break;

    case "array":
      if (prop.items) {
        const itemSchema = propertyToZod(prop.items);
        field = itemSchema ? z.array(itemSchema) : z.array(z.unknown());
      } else {
        field = z.array(z.unknown());
      }
      if (prop.minItems != null) field = (field as z.ZodArray<ZodTypeAny>).min(prop.minItems);
      if (prop.maxItems != null) field = (field as z.ZodArray<ZodTypeAny>).max(prop.maxItems);
      break;

    case "object":
      if (prop.properties) {
        const nested = schemaToZod(prop.properties);
        field = nested ?? z.record(z.unknown());
      } else {
        field = z.record(z.unknown());
      }
      break;

    default:
      return null;
  }

  // All fields are optional by default in config schemas
  return field.optional();
}

/**
 * React hook that memoizes the JSON Schema → Zod conversion.
 */
export function useSchemaToZod(
  properties: Record<string, PropertySchema> | undefined,
): z.ZodObject<Record<string, ZodTypeAny>> | null {
  return useMemo(() => schemaToZod(properties), [properties]);
}
