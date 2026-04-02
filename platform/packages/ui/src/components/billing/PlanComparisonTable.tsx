import type { BillingPlan } from "../../types/api";
import { cn } from "@neokapi/ui-primitives";
import { Check, X } from "lucide-react";

export interface ComparisonFeature {
  label: string;
  values: Record<BillingPlan, string | boolean>;
}

export interface PlanComparisonTableProps {
  features: ComparisonFeature[];
  recommendedPlan?: BillingPlan;
  className?: string;
}

const planLabels: Record<BillingPlan, string> = {
  free: "Free",
  pro: "Pro",
  team: "Team",
  enterprise: "Enterprise",
};

const plans: BillingPlan[] = ["free", "pro", "team", "enterprise"];

function CellValue({ value }: { value: string | boolean }) {
  if (typeof value === "boolean") {
    return value ? (
      <Check className="mx-auto h-4 w-4 text-green-500" />
    ) : (
      <X className="mx-auto h-4 w-4 text-muted-foreground/40" />
    );
  }
  return <span className="text-sm">{value}</span>;
}

export function PlanComparisonTable({
  features,
  recommendedPlan = "pro",
  className,
}: PlanComparisonTableProps) {
  return (
    <div className={cn("overflow-x-auto", className)}>
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border">
            <th className="py-3 pr-4 text-left font-medium text-muted-foreground">Feature</th>
            {plans.map((p) => (
              <th
                key={p}
                className={cn(
                  "px-4 py-3 text-center font-semibold",
                  p === recommendedPlan
                    ? "bg-primary/5 text-primary dark:bg-primary/10"
                    : "text-foreground",
                )}
              >
                {planLabels[p]}
                {p === recommendedPlan && (
                  <div className="mt-0.5 text-[10px] font-normal text-primary">Recommended</div>
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {features.map((f) => (
            <tr key={f.label} className="border-b border-border/50">
              <td className="py-2.5 pr-4 text-foreground">{f.label}</td>
              {plans.map((p) => (
                <td
                  key={p}
                  className={cn(
                    "px-4 py-2.5 text-center",
                    p === recommendedPlan && "bg-primary/5 dark:bg-primary/10",
                  )}
                >
                  <CellValue value={f.values[p]} />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
