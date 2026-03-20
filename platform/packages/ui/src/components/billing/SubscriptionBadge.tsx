import type { BillingPlan, BillingStatus } from "../../types/api";
import { cn } from "../../lib/utils";

export interface SubscriptionBadgeProps {
  plan: BillingPlan;
  status?: BillingStatus;
  className?: string;
}

const planColors: Record<BillingPlan, string> = {
  free: "bg-muted text-muted-foreground",
  pro: "bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300",
  team: "bg-purple-100 text-purple-800 dark:bg-purple-900/40 dark:text-purple-300",
  enterprise: "bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300",
};

const statusColors: Record<BillingStatus, string> = {
  active: "bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300",
  trialing: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/40 dark:text-yellow-300",
  past_due: "bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300",
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
