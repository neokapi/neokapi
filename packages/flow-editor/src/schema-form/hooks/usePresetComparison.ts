import { useMemo } from "react";

export function usePresetComparison(
  name: string,
  value: unknown,
  defaultValue: unknown,
  presetValues: Record<string, unknown> | undefined,
): boolean {
  return useMemo(() => {
    if (!presetValues) return false;
    const presetVal = presetValues[name];
    const currentVal = value ?? defaultValue;
    if (presetVal === undefined && currentVal === undefined) return false;
    return JSON.stringify(currentVal) !== JSON.stringify(presetVal);
  }, [name, value, defaultValue, presetValues]);
}
