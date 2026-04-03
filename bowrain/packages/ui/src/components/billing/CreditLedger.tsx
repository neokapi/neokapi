import type { CreditLedgerEntry } from "../../types/api";
import { cn } from "@neokapi/ui-primitives";
import { useState } from "react";

export interface CreditLedgerProps {
  entries: CreditLedgerEntry[];
  className?: string;
}

const operationLabels: Record<string, string> = {
  ai_translation: "AI Translation",
  ai_quality_check: "AI Quality Check",
  bravo_message: "@bravo Message",
  bravo_container: "@bravo Container",
  purchase: "Credit Purchase",
  grant: "Credit Grant",
  expire: "Expired",
  plan_reset: "Weekly Reset",
};

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatAmount(n: number): string {
  const abs = Math.abs(n);
  if (abs >= 1_000_000) return `${(abs / 1_000_000).toFixed(1)}M`;
  if (abs >= 1_000) return `${(abs / 1_000).toFixed(1)}K`;
  return String(abs);
}

export function CreditLedger({ entries, className }: CreditLedgerProps) {
  const operations = Array.from(new Set(entries.map((e) => e.operation)));
  const [filter, setFilter] = useState<string>("all");

  const filtered = filter === "all" ? entries : entries.filter((e) => e.operation === filter);

  return (
    <div className={cn("space-y-3", className)}>
      {operations.length > 1 && (
        <div className="flex flex-wrap gap-1">
          <button
            type="button"
            onClick={() => setFilter("all")}
            className={cn(
              "rounded-full px-2.5 py-0.5 text-xs font-medium transition-colors",
              filter === "all"
                ? "bg-primary text-primary-foreground"
                : "bg-muted text-muted-foreground hover:bg-muted/80",
            )}
          >
            All
          </button>
          {operations.map((op) => (
            <button
              key={op}
              type="button"
              onClick={() => setFilter(op)}
              className={cn(
                "rounded-full px-2.5 py-0.5 text-xs font-medium transition-colors",
                filter === op
                  ? "bg-primary text-primary-foreground"
                  : "bg-muted text-muted-foreground hover:bg-muted/80",
              )}
            >
              {operationLabels[op] ?? op}
            </button>
          ))}
        </div>
      )}
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left">
              <th className="py-2 pr-4 font-medium text-muted-foreground">Date</th>
              <th className="py-2 pr-4 font-medium text-muted-foreground">Operation</th>
              <th className="py-2 pr-4 text-right font-medium text-muted-foreground">Amount</th>
              <th className="py-2 pr-4 text-right font-medium text-muted-foreground">Balance</th>
              <th className="py-2 font-medium text-muted-foreground">Reference</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((entry) => (
              <tr key={entry.id} className="border-b border-border/50">
                <td className="py-2 pr-4 text-xs text-muted-foreground">
                  {formatDate(entry.createdAt)}
                </td>
                <td className="py-2 pr-4 text-foreground">
                  {operationLabels[entry.operation] ?? entry.operation}
                </td>
                <td
                  className={cn(
                    "py-2 pr-4 text-right font-mono text-xs font-medium",
                    entry.amount > 0
                      ? "text-green-600 dark:text-green-400"
                      : "text-red-600 dark:text-red-400",
                  )}
                >
                  {entry.amount > 0 ? "+" : "-"}
                  {formatAmount(entry.amount)}
                </td>
                <td className="py-2 pr-4 text-right font-mono text-xs text-muted-foreground">
                  {formatAmount(entry.balanceAfter)}
                </td>
                <td className="py-2 text-xs text-muted-foreground">
                  {entry.referenceId ? (
                    <span className="font-mono">{entry.referenceId.slice(0, 8)}</span>
                  ) : (
                    <span className="italic">--</span>
                  )}
                </td>
              </tr>
            ))}
            {filtered.length === 0 && (
              <tr>
                <td colSpan={5} className="py-6 text-center text-sm text-muted-foreground">
                  No transactions found
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
