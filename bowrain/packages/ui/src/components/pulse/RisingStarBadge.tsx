import { TrendingUp, TrendingDown } from "lucide-react";

interface RisingStarBadgeProps {
  growth: number;
  className?: string;
}

export function RisingStarBadge({ growth, className }: RisingStarBadgeProps) {
  const isPositive = growth > 0;
  const Icon = isPositive ? TrendingUp : TrendingDown;
  const color = isPositive ? "text-success bg-success/5" : "text-destructive bg-destructive/5";

  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${color} ${className ?? ""}`}
    >
      <Icon className="h-3 w-3" />
      {isPositive ? "+" : ""}
      {growth.toFixed(1)}%
    </span>
  );
}
