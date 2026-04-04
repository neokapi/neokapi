import { cn } from "@neokapi/ui-primitives";

export interface UsageBarProps {
  creditsUsed: number;
  creditsTotal: number;
  weekEnd: Date;
  className?: string;
}

function formatCountdown(weekEnd: Date): string {
  const now = new Date();
  const diff = weekEnd.getTime() - now.getTime();
  if (diff <= 0) return "Resetting now";
  const days = Math.floor(diff / (1000 * 60 * 60 * 24));
  const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
  if (days > 0) return `Resets in ${days}d ${hours}h`;
  if (hours > 0) return `Resets in ${hours}h`;
  const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
  return `Resets in ${minutes}m`;
}

function formatCredits(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`;
  return String(n);
}

export function UsageBar({ creditsUsed, creditsTotal, weekEnd, className }: UsageBarProps) {
  const pct = creditsTotal > 0 ? Math.min((creditsUsed / creditsTotal) * 100, 100) : 0;

  const barColor =
    pct > 80
      ? "bg-destructive dark:bg-destructive"
      : pct > 60
        ? "bg-warning dark:bg-warning"
        : "bg-success dark:bg-success";

  return (
    <div className={cn("space-y-1.5", className)}>
      <div className="flex items-center justify-between text-xs">
        <span className="font-medium text-foreground">
          {formatCredits(creditsUsed)} / {formatCredits(creditsTotal)} credits
        </span>
        <span className="text-muted-foreground">{Math.round(pct)}%</span>
      </div>
      <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
        <div
          className={cn("h-full rounded-full transition-all duration-300", barColor)}
          style={{ width: `${pct}%` }}
        />
      </div>
      <div className="text-xs text-muted-foreground">{formatCountdown(weekEnd)}</div>
    </div>
  );
}
