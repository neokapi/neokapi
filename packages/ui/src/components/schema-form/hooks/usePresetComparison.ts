import { useMemo } from "react";

export function usePresetComparison(
  name: string,
  value: unknown,
  defaultValue: unknown,
  presetValues?: Record<string, unknown>,
): boolean {
  return useMemo(() => {
    if (!presetValues) return false;
    const presetValue = presetValues[name];
    if (presetValue === undefined) return false;
    const current = value ?? defaultValue;
    return JSON.stringify(current) !== JSON.stringify(presetValue);
  }, [name, value, defaultValue, presetValues]);
}
