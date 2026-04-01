/**
 * useValidation — runs Zod validation on form values and returns field-level errors.
 * Integrates with the existing controlled values/onChange pattern without
 * requiring React Hook Form's form context throughout the component tree.
 */
import { useMemo } from "react";
import type { ZodObject, ZodTypeAny } from "zod";

export interface FieldErrors {
  [fieldName: string]: string | undefined;
}

/**
 * Validate form values against a Zod schema.
 * Returns a map of field names to error messages.
 */
export function useValidation(
  zodSchema: ZodObject<Record<string, ZodTypeAny>> | null,
  values: Record<string, unknown>,
): FieldErrors {
  return useMemo(() => {
    if (!zodSchema) return {};

    try {
      const result = zodSchema.safeParse(values);
      if (result.success) return {};

      const errors: FieldErrors = {};
      for (const issue of result.error.issues) {
        const path = issue.path.join(".");
        if (path && !errors[path]) {
          errors[path] = issue.message;
        }
      }
      return errors;
    } catch {
      // Zod instance mismatch (e.g., Storybook prebundling creates a
      // separate copy). Validation is best-effort — degrade gracefully.
      return {};
    }
  }, [zodSchema, values]);
}
