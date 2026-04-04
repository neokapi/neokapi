import { cn } from "@neokapi/ui-primitives";

export interface CreditCounterProps {
  creditsUsed: number;
  creditsTotal: number;
  compact?: boolean;
  className?: string;
}

function formatCredits(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`;
  return String(n);
}

export function CreditCounter({
  creditsUsed,
  creditsTotal,
  compact = false,
  className,
}: CreditCounterProps) {
  const remaining = Math.max(creditsTotal - creditsUsed, 0);
  const pct = creditsTotal > 0 ? Math.min((creditsUsed / creditsTotal) * 100, 100) : 0;

  const ringColor =
    pct > 80
      ? "text-destructive dark:text-destructive"
      : pct > 60
        ? "text-warning dark:text-warning"
        : "text-success dark:text-success";

  if (compact) {
    return (
      <span
        className={cn(
          "inline-flex items-center gap-1 text-xs font-medium text-muted-foreground",
          className,
        )}
      >
        <span
          className={cn("inline-block h-2 w-2 rounded-full", {
            "bg-success dark:bg-success": pct <= 60,
            "bg-warning dark:bg-warning": pct > 60 && pct <= 80,
            "bg-destructive dark:bg-destructive": pct > 80,
          })}
        />
        {formatCredits(remaining)}
      </span>
    );
  }

  return (
    <div className={cn("inline-flex items-center gap-2", className)}>
      <svg className={cn("h-5 w-5 -rotate-90", ringColor)} viewBox="0 0 20 20">
        <circle
          cx="10"
          cy="10"
          r="8"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          opacity="0.2"
        />
        <circle
          cx="10"
          cy="10"
          r="8"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeDasharray={`${(pct / 100) * 50.27} 50.27`}
          strokeLinecap="round"
        />
      </svg>
      <span className="text-sm font-medium text-foreground">
        {formatCredits(remaining)} / {formatCredits(creditsTotal)}
      </span>
    </div>
  );
}
