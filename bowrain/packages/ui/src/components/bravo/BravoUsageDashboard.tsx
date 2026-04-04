import { useState } from "react";
import type { BravoUsageSummary } from "../../types/api";

export interface BravoUsageDashboardProps {
  usage: BravoUsageSummary;
  /** Optional date range controls. */
  onDateRangeChange?: (from: string, to: string) => void;
}

function StatCard({ label, value, sublabel }: { label: string; value: string; sublabel?: string }) {
  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="text-2xl font-bold tracking-tight">{value}</div>
      <div className="text-sm font-medium text-muted-foreground mt-1">{label}</div>
      {sublabel && <div className="text-xs text-muted-foreground mt-0.5">{sublabel}</div>}
    </div>
  );
}

function TokenBar({
  label,
  value,
  total,
  color,
}: {
  label: string;
  value: number;
  total: number;
  color: string;
}) {
  const pct = total > 0 ? Math.round((value / total) * 100) : 0;
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-xs">
        <span className="font-medium">{label}</span>
        <span className="text-muted-foreground">
          {(value / 1000).toFixed(1)}k ({pct}%)
        </span>
      </div>
      <div className="h-2 rounded-full bg-muted overflow-hidden">
        <div className={`h-full rounded-full ${color}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

export function BravoUsageDashboard({ usage, onDateRangeChange }: BravoUsageDashboardProps) {
  const [dateRange, setDateRange] = useState<"7d" | "30d" | "90d">("30d");

  const handleRangeChange = (range: "7d" | "30d" | "90d") => {
    setDateRange(range);
    if (onDateRangeChange) {
      const now = new Date();
      const from = new Date(now);
      const days = range === "7d" ? 7 : range === "30d" ? 30 : 90;
      from.setDate(from.getDate() - days);
      onDateRangeChange(from.toISOString(), now.toISOString());
    }
  };

  const totalTokens = usage.total_input_tokens + usage.total_output_tokens;
  const containerMinutes = Math.ceil(usage.total_container_sec / 60);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold">@bravo Usage</h3>
        {onDateRangeChange && (
          <div className="flex gap-1">
            {(["7d", "30d", "90d"] as const).map((r) => (
              <button
                key={r}
                onClick={() => handleRangeChange(r)}
                className={`px-2 py-1 text-xs rounded-md transition-colors ${
                  dateRange === r
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-accent"
                }`}
              >
                {r}
              </button>
            ))}
          </div>
        )}
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard
          label="Total tokens"
          value={`${(totalTokens / 1000).toFixed(1)}k`}
          sublabel={`${totalTokens.toLocaleString()} tokens`}
        />
        <StatCard label="Messages" value={usage.message_count.toLocaleString()} />
        <StatCard
          label="Container time"
          value={`${containerMinutes}m`}
          sublabel={`${usage.total_container_sec.toLocaleString()}s`}
        />
        <StatCard
          label="Avg tokens/msg"
          value={
            usage.message_count > 0
              ? `${Math.round(totalTokens / usage.message_count).toLocaleString()}`
              : "0"
          }
        />
      </div>

      <div className="space-y-3">
        <h4 className="text-xs uppercase text-muted-foreground font-medium">Token breakdown</h4>
        <TokenBar
          label="Input tokens"
          value={usage.total_input_tokens}
          total={totalTokens}
          color="bg-info"
        />
        <TokenBar
          label="Output tokens"
          value={usage.total_output_tokens}
          total={totalTokens}
          color="bg-success"
        />
      </div>
    </div>
  );
}
