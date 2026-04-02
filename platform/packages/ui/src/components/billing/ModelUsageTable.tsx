import type { ModelUsage, RunnerUsage } from "../../types/api";
import { cn } from "@neokapi/ui-primitives";

export interface ModelUsageTableProps {
  entries: ModelUsage[];
  runnerEntries?: RunnerUsage[];
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

function formatDuration(seconds: number): string {
  if (seconds >= 3600) return `${(seconds / 3600).toFixed(1)}h`;
  if (seconds >= 60) return `${(seconds / 60).toFixed(1)}m`;
  return `${Math.round(seconds)}s`;
}

const runnerOperationLabels: Record<string, string> = {
  bravo_container: "@bravo Container",
  auto_translate: "Auto-Translate",
  auto_extract: "Auto-Extract",
  auto_translate_new_locale: "Auto-Translate New Locale",
  automation: "Automation",
};

export function ModelUsageTable({ entries, runnerEntries, className }: ModelUsageTableProps) {
  const hasTokens = entries.length > 0;
  const hasRunner = (runnerEntries ?? []).length > 0;

  if (!hasTokens && !hasRunner) {
    return (
      <p className={cn("text-sm text-muted-foreground", className)}>
        No AI usage recorded this period
      </p>
    );
  }

  return (
    <div className={cn("space-y-4", className)}>
      {hasTokens && (
        <div className="overflow-x-auto">
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
      )}

      {hasRunner && (
        <div className="overflow-x-auto">
          <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wide mb-2">
            Runner Time
          </h4>
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-muted-foreground">
                <th className="pb-2 pr-4 font-medium">Operation</th>
                <th className="pb-2 pr-4 font-medium text-right">Total Time</th>
                <th className="pb-2 font-medium text-right">Runs</th>
              </tr>
            </thead>
            <tbody>
              {(runnerEntries ?? []).map((ru, i) => (
                <tr key={i} className="border-b border-border/50 last:border-b-0">
                  <td className="py-2 pr-4 text-foreground">
                    {runnerOperationLabels[ru.operation] ?? ru.operation}
                  </td>
                  <td className="py-2 pr-4 font-mono text-right text-xs font-medium">
                    {formatDuration(ru.total_seconds)}
                  </td>
                  <td className="py-2 font-mono text-right text-xs text-muted-foreground">
                    {ru.count}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
