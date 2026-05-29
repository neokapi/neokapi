import { cn } from "@neokapi/ui-primitives";
import type { BlastRadius } from "./types";

interface BlastRadiusSummaryProps {
  radius: BlastRadius;
  className?: string;
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone?: "neutral" | "good" | "bad";
}) {
  const toneClass =
    tone === "good" ? "text-success" : tone === "bad" ? "text-destructive" : "text-foreground";
  return (
    <div className="rounded-md border p-2 text-center bg-card/50">
      <div className={cn("text-lg font-semibold tabular-nums", toneClass)}>{value}</div>
      <div className="text-[10px] text-muted-foreground">{label}</div>
    </div>
  );
}

/**
 * BlastRadiusSummary shows what promoting a candidate rule would do across
 * existing content — the number a reviewer sees before the rule lands: how many
 * blocks it would newly flag, what it resolves, and the per-collection spread.
 */
export function BlastRadiusSummary({ radius, className }: BlastRadiusSummaryProps) {
  return (
    <div className={cn("space-y-3", className)}>
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
        <Stat label="blocks" value={radius.total_blocks} />
        <Stat label="affected" value={radius.affected_blocks} />
        <Stat
          label="new violations"
          value={radius.new_violations}
          tone={radius.new_violations > 0 ? "bad" : "neutral"}
        />
        <Stat
          label="resolved"
          value={radius.resolved_violations}
          tone={radius.resolved_violations > 0 ? "good" : "neutral"}
        />
        <Stat
          label="critical"
          value={radius.critical_count}
          tone={radius.critical_count > 0 ? "bad" : "neutral"}
        />
        <Stat
          label="degraded"
          value={radius.degraded_blocks}
          tone={radius.degraded_blocks > 0 ? "bad" : "neutral"}
        />
      </div>

      {radius.collections.length > 0 && (
        <div className="space-y-1">
          <p className="text-xs font-medium text-muted-foreground">By collection</p>
          <ul className="space-y-1">
            {radius.collections.map((c) => (
              <li
                key={c.collection_id}
                className="flex items-center justify-between rounded border px-2 py-1 text-xs bg-card/50"
              >
                <span className="truncate">{c.collection_name || c.collection_id}</span>
                <span className="flex gap-3 tabular-nums text-muted-foreground">
                  <span>{c.affected_blocks} affected</span>
                  <span className={cn(c.avg_score_delta < 0 ? "text-destructive" : "text-success")}>
                    {c.avg_score_delta > 0 ? "+" : ""}
                    {c.avg_score_delta.toFixed(1)} avg
                  </span>
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
