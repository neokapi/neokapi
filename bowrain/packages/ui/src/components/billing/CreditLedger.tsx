import type { CreditLedgerEntry } from "../../types/api";
import { Button, cn } from "@neokapi/ui-primitives";

/** The chip value that clears the operation filter. */
export const ALL_OPERATIONS = "all";

export interface CreditLedgerProps {
  /** The current page of entries, already narrowed by `operation` server-side. */
  entries: CreditLedgerEntry[];
  /**
   * The operations the chip row offers. A caller reading a page hands the
   * server's whole-window operation set, so the chips stay put as the pages
   * turn; a caller holding a complete ledger may derive them from its rows.
   */
  operations?: string[];
  /** The selected operation, or {@link ALL_OPERATIONS}. */
  operation?: string;
  onOperationChange?: (operation: string) => void;
  /** Entries the current filter matches over the whole window. */
  total?: number;
  limit?: number;
  offset?: number;
  onOffsetChange?: (offset: number) => void;
  loading?: boolean;
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

export function operationLabel(operation: string): string {
  return operationLabels[operation] ?? operation;
}

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

function chipClass(active: boolean): string {
  return cn(
    "rounded-full px-2.5 py-0.5 text-xs font-medium transition-colors",
    active
      ? "bg-primary text-primary-foreground"
      : "bg-muted text-muted-foreground hover:bg-muted/80",
  );
}

export function CreditLedger({
  entries,
  operations = [],
  operation = ALL_OPERATIONS,
  onOperationChange,
  total = entries.length,
  limit = 0,
  offset = 0,
  onOffsetChange,
  loading,
  className,
}: CreditLedgerProps) {
  const pageEnd = offset + entries.length;
  const canPage = limit > 0 && onOffsetChange !== undefined;
  const hasPrev = canPage && offset > 0;
  const hasNext = canPage && pageEnd < total;

  return (
    <div className={cn("space-y-3", className)}>
      {operations.length > 1 && onOperationChange && (
        <div className="flex flex-wrap gap-1">
          <button
            type="button"
            onClick={() => onOperationChange(ALL_OPERATIONS)}
            className={chipClass(operation === ALL_OPERATIONS)}
          >
            All
          </button>
          {operations.map((op) => (
            <button
              key={op}
              type="button"
              onClick={() => onOperationChange(op)}
              className={chipClass(operation === op)}
            >
              {operationLabel(op)}
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
            {entries.map((entry) => (
              <tr key={entry.id} className="border-b border-border/50">
                <td className="py-2 pr-4 text-xs text-muted-foreground">
                  {formatDate(entry.createdAt)}
                </td>
                <td className="py-2 pr-4 text-foreground">{operationLabel(entry.operation)}</td>
                <td
                  className={cn(
                    "py-2 pr-4 text-right font-mono text-xs font-medium",
                    entry.amount > 0
                      ? "text-success dark:text-success"
                      : "text-destructive dark:text-destructive",
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
            {entries.length === 0 && (
              <tr>
                <td colSpan={5} className="py-6 text-center text-sm text-muted-foreground">
                  {loading ? "Loading transactions…" : "No transactions found"}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      {(hasPrev || hasNext) && (
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground" data-testid="ledger-range">
            {offset + 1}–{pageEnd} of {total}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={!hasPrev || loading}
              onClick={() => onOffsetChange?.(Math.max(0, offset - limit))}
              data-testid="ledger-prev"
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={!hasNext || loading}
              onClick={() => onOffsetChange?.(offset + limit)}
              data-testid="ledger-next"
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
