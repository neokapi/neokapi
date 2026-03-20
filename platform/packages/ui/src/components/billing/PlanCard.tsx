import type { BillingPlan } from "../../types/api";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
import { Card, CardContent, CardFooter, CardHeader, CardTitle, CardDescription } from "../ui/card";
import { Check, X } from "lucide-react";

export interface PlanFeature {
  label: string;
  included: boolean;
}

export interface PlanCardProps {
  plan: BillingPlan;
  name: string;
  price: string;
  period?: string;
  description?: string;
  credits: string;
  features: PlanFeature[];
  recommended?: boolean;
  current?: boolean;
  ctaLabel?: string;
  onSelect?: () => void;
  className?: string;
}

const planAccent: Record<BillingPlan, string> = {
  free: "ring-border",
  pro: "ring-blue-500 dark:ring-blue-400",
  team: "ring-purple-500 dark:ring-purple-400",
  enterprise: "ring-amber-500 dark:ring-amber-400",
};

export function PlanCard({
  plan,
  name,
  price,
  period,
  description,
  credits,
  features,
  recommended = false,
  current = false,
  ctaLabel,
  onSelect,
  className,
}: PlanCardProps) {
  return (
    <Card
      className={cn(
        "relative flex flex-col",
        recommended && `ring-2 ${planAccent[plan]}`,
        className,
      )}
    >
      {recommended && (
        <div
          className={cn(
            "absolute -top-3 left-1/2 -translate-x-1/2 rounded-full px-3 py-0.5 text-xs font-semibold text-white",
            plan === "pro" ? "bg-blue-500" : plan === "team" ? "bg-purple-500" : "bg-primary",
          )}
        >
          Recommended
        </div>
      )}
      <CardHeader>
        <CardTitle className="text-lg">{name}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent className="flex-1 space-y-4">
        <div>
          <span className="text-3xl font-bold text-foreground">{price}</span>
          {period && <span className="ml-1 text-sm text-muted-foreground">/{period}</span>}
        </div>
        <div className="text-sm text-muted-foreground">{credits}</div>
        <ul className="space-y-2">
          {features.map((f) => (
            <li key={f.label} className="flex items-start gap-2 text-sm">
              {f.included ? (
                <Check className="mt-0.5 h-4 w-4 shrink-0 text-green-500" />
              ) : (
                <X className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground/40" />
              )}
              <span className={f.included ? "text-foreground" : "text-muted-foreground"}>
                {f.label}
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
      <CardFooter>
        <Button
          className="w-full"
          variant={current ? "outline" : "default"}
          disabled={current}
          onClick={onSelect}
        >
          {current ? "Current Plan" : (ctaLabel ?? "Select")}
        </Button>
      </CardFooter>
    </Card>
  );
}
