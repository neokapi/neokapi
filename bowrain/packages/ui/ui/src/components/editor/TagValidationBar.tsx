import type { TagValidationResult } from "./tagSemantics";
import { AlertTriangle, Info } from "../icons";

interface TagValidationBarProps {
  validation: TagValidationResult | null;
}

/**
 * Compact bar displaying tag validation errors and warnings.
 * Red row for errors (missing tags, unpaired tags), yellow for warnings (extra tags).
 */
export function TagValidationBar({ validation }: TagValidationBarProps) {
  if (!validation || (validation.errors.length === 0 && validation.warnings.length === 0)) {
    return null;
  }

  return (
    <div className="flex flex-col gap-0.5 mt-1">
      {validation.errors.map((err, i) => (
        <div key={`e-${i}`} className="text-[11px] px-2 py-0.5 rounded-sm flex items-center gap-1 bg-destructive/10 text-destructive border border-destructive/25">
          <AlertTriangle className="w-3 h-3 shrink-0" />
          {err.message}
        </div>
      ))}
      {validation.warnings.map((warn, i) => (
        <div key={`w-${i}`} className="text-[11px] px-2 py-0.5 rounded-sm flex items-center gap-1 bg-amber-500/10 text-amber-700 dark:text-amber-400 border border-amber-500/25">
          <Info className="w-3 h-3 shrink-0" />
          {warn.message}
        </div>
      ))}
    </div>
  );
}
