import type { PropertySchema } from "../types";

export function resolveSchemaRef(schema: PropertySchema, defs: Record<string, PropertySchema> | undefined): PropertySchema {
  if (!schema.$ref || !defs) return schema;
  // Handle "#/$defs/name" format
  const match = schema.$ref.match(/^#\/\$defs\/(.+)$/);
  if (!match) return schema;
  const resolved = defs[match[1]];
  if (!resolved) return schema;
  // Merge: the referencing schema's own fields override the $def
  const { $ref: _, ...rest } = schema;
  return { ...resolved, ...rest };
}
