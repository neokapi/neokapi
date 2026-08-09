import type { DimensionScore } from "./types";
import { cn } from "@neokapi/ui-primitives";
import { DEFAULT_MIN_SCORE, scoreFillClass } from "./complianceBar";

interface BrandDimensionBreakdownProps {
  dimensions: DimensionScore[];
  /** The profile's on-brand bar; DEFAULT_MIN_SCORE when no profile is loaded. */
  bar?: number;
  className?: string;
}

const dimensionLabels: Record<string, string> = {
  tone: "Tone",
  style: "Style",
  vocabulary: "Vocabulary",
  clarity: "Clarity",
  brand_compliance: "Brand",
};

export function BrandDimensionBreakdown({
  dimensions,
  bar = DEFAULT_MIN_SCORE,
  className,
}: BrandDimensionBreakdownProps) {
  return (
    <div className={cn("space-y-3", className)}>
      {dimensions.map((dim) => (
        <div key={dim.dimension} className="space-y-1">
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">
              {dimensionLabels[dim.dimension] ?? dim.dimension}
            </span>
            <span
              className="font-medium tabular-nums"
              data-testid={`dimension-score-${dim.dimension}`}
              data-below-bar={dim.score < bar}
            >
              {dim.score}
            </span>
          </div>
          <div className="h-2 rounded-full bg-muted overflow-hidden">
            <div
              className={cn(
                "h-full rounded-full transition-all duration-500",
                scoreFillClass(dim.score, bar),
              )}
              style={{ width: `${dim.score}%` }}
            />
          </div>
          {dim.issues > 0 && (
            <span className="text-[10px] text-muted-foreground">
              {dim.issues} issue{dim.issues !== 1 ? "s" : ""}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}
