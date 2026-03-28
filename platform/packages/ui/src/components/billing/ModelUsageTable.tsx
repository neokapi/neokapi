import type { ModelUsage } from "../../types/api";
import { cn } from "../../lib/utils";

export interface ModelUsageTableProps {
  entries: ModelUsage[];
  className?: string;
}

const operationLabels: Record<string, string> = {
  translate: "Translation",
  qa_check: "Quality Check",
  review: "Review",
  entity_extract: "Entity Extraction",
  terminology: "Terminology",
  brand_voice: "Brand Voice",
};

function formatTokens(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(0)}K`;
  return String(value);
}

export function ModelUsageTable({ entries, className }: ModelUsageTableProps) {
  if (entries.length === 0) {
    return (
      <p className={cn("text-sm text-muted-foreground", className)}>
        No AI usage recorded this period
      </p>
    );
  }

  return (
    <div className={cn("overflow-x-auto", className)}>
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left text-muted-foreground">
            <th className="pb-2 pr-4 font-medium">Model</th>
            <th className="pb-2 pr-4 font-medium">Operation</th>
            <th className="pb-2 pr-4 font-medium text-right">Input</th>
            <th className="pb-2 pr-4 font-medium text-right">Output</th>
            <th className="pb-2 pr-4 font-medium text-right">Total</th>
            <th className="pb-2 font-medium text-right">Calls</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((mu, i) => (
            <tr key={i} className="border-b border-border/50 last:border-b-0">
              <td className="py-2 pr-4 font-mono text-xs">{mu.model}</td>
              <td className="py-2 pr-4 text-muted-foreground">
                {operationLabels[mu.operation] ?? mu.operation}
              </td>
              <td className="py-2 pr-4 font-mono text-right text-xs">
                {formatTokens(mu.prompt_tokens)}
              </td>
              <td className="py-2 pr-4 font-mono text-right text-xs">
                {formatTokens(mu.output_tokens)}
              </td>
              <td className="py-2 pr-4 font-mono text-right text-xs font-medium">
                {formatTokens(mu.total_tokens)}
              </td>
              <td className="py-2 font-mono text-right text-xs text-muted-foreground">
                {mu.call_count}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
