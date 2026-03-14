import type { DimensionScore } from "./types";
import { cn } from "../lib/utils";

interface BrandDimensionBreakdownProps {
  dimensions: DimensionScore[];
  className?: string;
}

function barColor(score: number): string {
  if (score >= 80) return "bg-green-500";
  if (score >= 60) return "bg-yellow-500";
  if (score >= 40) return "bg-orange-500";
  return "bg-red-500";
}

const dimensionLabels: Record<string, string> = {
  tone: "Tone",
  style: "Style",
  vocabulary: "Vocabulary",
  clarity: "Clarity",
  brand_compliance: "Brand",
};

export function BrandDimensionBreakdown({ dimensions, className }: BrandDimensionBreakdownProps) {
  return (
    <div className={cn("space-y-3", className)}>
      {dimensions.map((dim) => (
        <div key={dim.dimension} className="space-y-1">
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">
              {dimensionLabels[dim.dimension] ?? dim.dimension}
            </span>
            <span className="font-medium tabular-nums">{dim.score}</span>
          </div>
          <div className="h-2 rounded-full bg-muted overflow-hidden">
            <div
              className={cn("h-full rounded-full transition-all duration-500", barColor(dim.score))}
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
