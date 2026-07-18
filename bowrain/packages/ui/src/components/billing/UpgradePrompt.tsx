import { Button, Card, CardContent, cn } from "@neokapi/ui-primitives";
import type { BillingPlan } from "../../types/api";
import { ArrowUpRight } from "lucide-react";

export interface UpgradePromptProps {
  feature: string;
  minimumPlan: BillingPlan;
  currentPlan: BillingPlan;
  onUpgrade?: () => void;
  className?: string;
}

const planLabels: Record<BillingPlan, string> = {
  free: "Free",
  pro: "Pro",
  team: "Team",
  enterprise: "Enterprise",
};

// Only capabilities the product actually has and gates (AD-018: a highlight
// nothing enforces is a promise the code does not keep). Credit numbers mirror
// billing/plans.go MonthlyCredits.
const planHighlights: Record<BillingPlan, string[]> = {
  free: [],
  pro: ["2M monthly credits", "Git connectors", "API access"],
  team: ["8M monthly credits", "Unlimited seats", "Priority support"],
  enterprise: ["Custom credits", "SSO/SAML", "Dedicated support", "Custom agreements"],
};

export function UpgradePrompt({
  feature,
  minimumPlan,
  currentPlan,
  onUpgrade,
  className,
}: UpgradePromptProps) {
  const highlights = planHighlights[minimumPlan];

  return (
    <Card className={cn("border-primary/20 bg-primary/5 dark:bg-primary/10", className)}>
      <CardContent className="space-y-3">
        <div className="text-sm font-medium text-foreground">
          {feature} requires a {planLabels[minimumPlan]} plan
        </div>
        <p className="text-xs text-muted-foreground">
          You are currently on the {planLabels[currentPlan]} plan. Upgrade to{" "}
          {planLabels[minimumPlan]} to unlock:
        </p>
        {highlights.length > 0 && (
          <ul className="space-y-1">
            {highlights.map((h) => (
              <li key={h} className="flex items-center gap-1.5 text-xs text-foreground">
                <ArrowUpRight className="h-3 w-3 text-primary" />
                {h}
              </li>
            ))}
          </ul>
        )}
        <Button size="sm" onClick={onUpgrade}>
          Upgrade to {planLabels[minimumPlan]}
        </Button>
      </CardContent>
    </Card>
  );
}
