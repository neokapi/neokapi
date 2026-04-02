import type { BillingPlan } from "../../types/api";
import { cn } from "@neokapi/ui-primitives";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { Card, CardContent } from "@neokapi/ui-primitives/components/ui/card";
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

const planHighlights: Record<BillingPlan, string[]> = {
  free: [],
  pro: ["500K weekly credits", "Git connectors", "API access", "Custom MT providers"],
  team: ["2M weekly credits", "Unlimited seats", "@bravo code execution", "Custom connectors"],
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
