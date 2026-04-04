import type { BillingPlan, BillingStatus } from "../../types/api";
import { cn } from "@neokapi/ui-primitives";

export interface SubscriptionBadgeProps {
  plan: BillingPlan;
  status?: BillingStatus;
  className?: string;
}

const planColors: Record<BillingPlan, string> = {
  free: "bg-muted text-muted-foreground",
  pro: "bg-info/10 text-info dark:bg-info/20 dark:text-info",
  team: "bg-purple-100 text-purple-800 dark:bg-purple-900/40 dark:text-purple-300",
  enterprise: "bg-warning/10 text-warning dark:bg-warning/20 dark:text-warning",
};

const statusColors: Record<BillingStatus, string> = {
  active: "bg-success/10 text-success dark:bg-success/20 dark:text-success",
  trialing: "bg-warning/10 text-warning dark:bg-warning/20 dark:text-warning",
  past_due: "bg-destructive/10 text-destructive dark:bg-destructive/20 dark:text-destructive",
  canceled: "bg-muted text-muted-foreground",
};

const planLabels: Record<BillingPlan, string> = {
  free: "Free",
  pro: "Pro",
  team: "Team",
  enterprise: "Enterprise",
};

const statusLabels: Record<BillingStatus, string> = {
  active: "Active",
  trialing: "Trial",
  past_due: "Past Due",
  canceled: "Canceled",
};

export function SubscriptionBadge({ plan, status, className }: SubscriptionBadgeProps) {
  return (
    <span className={cn("inline-flex items-center gap-1.5", className)}>
      <span
        className={cn(
          "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
          planColors[plan],
        )}
      >
        {planLabels[plan]}
      </span>
      {status && (
        <span
          className={cn(
            "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
            statusColors[status],
          )}
        >
          {statusLabels[status]}
        </span>
      )}
    </span>
  );
}
