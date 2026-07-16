import { Badge, cn } from "@neokapi/ui";
import type { Coverage } from "./locale-demand-fixtures";

export interface CoverageDescriptor {
  label: string;
  /** Tailwind classes for the badge tint. */
  className: string;
}

/** Pure mapping from coverage state to badge copy + tint (unit-tested). */
export function describeCoverage(coverage: Coverage): CoverageDescriptor {
  switch (coverage.status) {
    case "covered":
      return {
        label: "Covered",
        className: "border-transparent bg-success/15 text-success",
      };
    case "partial":
      return {
        label: `Partial ${coverage.percent}%`,
        className: "border-transparent bg-warning/15 text-warning",
      };
    case "not-covered":
      return {
        label: "Not covered",
        className: "border-transparent bg-muted text-muted-foreground",
      };
  }
}

export function CoverageBadge({ coverage, className }: { coverage: Coverage; className?: string }) {
  const { label, className: tint } = describeCoverage(coverage);
  return (
    <Badge variant="outline" className={cn(tint, className)} data-testid="coverage-badge">
      {label}
    </Badge>
  );
}
