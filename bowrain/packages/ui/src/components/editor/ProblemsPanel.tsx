import { useState, useMemo } from "react";
import type { FileQAResult, QAIssue } from "../../types/api";
import { X, Check, AlertTriangle, Info } from "../icons";

interface ProblemsPanelProps {
  issues: FileQAResult[];
  loading?: boolean;
  onNavigateToBlock: (blockId: string) => void;
  onClose: () => void;
}

type FilterMode = "all" | "errors";

interface FlatIssue {
  blockId: string;
  issue: QAIssue;
}

/**
 * ProblemsPanel slides up from the bottom to display QA check results,
 * similar to VS Code's problems panel.
 */
export function ProblemsPanel({ issues, loading, onNavigateToBlock, onClose }: ProblemsPanelProps) {
  const [filter, setFilter] = useState<FilterMode>("all");

  const flatIssues = useMemo(() => {
    const flat: FlatIssue[] = [];
    for (const result of issues) {
      for (const issue of result.issues) {
        flat.push({ blockId: result.blockId, issue });
      }
    }
    // Sort: errors first, then warnings.
    flat.sort((a, b) => {
      if (a.issue.severity === b.issue.severity) return 0;
      return a.issue.severity === "error" ? -1 : 1;
    });
    return flat;
  }, [issues]);

  const filtered = useMemo(() => {
    if (filter === "errors") {
      return flatIssues.filter((f) => f.issue.severity === "error");
    }
    return flatIssues;
  }, [flatIssues, filter]);

  const errorCount = useMemo(
    () => flatIssues.filter((f) => f.issue.severity === "error").length,
    [flatIssues],
  );
  const warningCount = useMemo(
    () => flatIssues.filter((f) => f.issue.severity === "warning").length,
    [flatIssues],
  );
  const totalCount = flatIssues.length;

  return (
    <div className="fixed bottom-0 left-0 right-0 z-50 border-t border-border/50 bg-card shadow-[0_-4px_24px_rgba(0,0,0,0.15)] dark:shadow-[0_-4px_24px_rgba(0,0,0,0.4)] flex flex-col max-h-[40vh]">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2 border-b border-border/30 shrink-0">
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-semibold text-foreground">Problems</h3>
          {!loading && (
            <span className="text-xs px-1.5 py-0.5 rounded-full bg-muted text-muted-foreground font-medium">
              {totalCount}
            </span>
          )}
          {!loading && totalCount > 0 && (
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <AlertTriangle className="w-3 h-3 text-destructive" />
              <span>{errorCount}</span>
              <Info className="w-3 h-3 text-amber-500 ml-1" />
              <span>{warningCount}</span>
            </div>
          )}
        </div>
        <div className="flex items-center gap-2">
          {/* Filter buttons */}
          <div className="flex items-center rounded-md border border-border/50 overflow-hidden">
            <button
              onClick={() => setFilter("all")}
              className={`px-2 py-0.5 text-xs transition-colors ${
                filter === "all"
                  ? "bg-primary/15 text-primary font-medium"
                  : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
              }`}
            >
              All
            </button>
            <button
              onClick={() => setFilter("errors")}
              className={`px-2 py-0.5 text-xs border-l border-border/50 transition-colors ${
                filter === "errors"
                  ? "bg-primary/15 text-primary font-medium"
                  : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
              }`}
            >
              Errors only
            </button>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/50 transition-colors"
            aria-label="Close problems panel"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="flex items-center justify-center py-8">
            <div className="animate-spin rounded-full h-5 w-5 border-2 border-primary border-t-transparent" />
            <span className="ml-2 text-sm text-muted-foreground">Running QA checks...</span>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-8 gap-2">
            <div className="rounded-full p-2 bg-success/10">
              <Check className="w-5 h-5 text-success" />
            </div>
            <span className="text-sm text-muted-foreground">No issues found</span>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-card">
              <tr className="text-left text-xs text-muted-foreground border-b border-border/30">
                <th className="px-4 py-1.5 font-medium w-[120px]">Block</th>
                <th className="px-4 py-1.5 font-medium w-[80px]">Severity</th>
                <th className="px-4 py-1.5 font-medium w-[140px]">Type</th>
                <th className="px-4 py-1.5 font-medium">Message</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((item, i) => (
                <tr
                  key={`${item.blockId}-${i}`}
                  onClick={() => onNavigateToBlock(item.blockId)}
                  className="border-b border-border/20 cursor-pointer hover:bg-muted/30 transition-colors"
                >
                  <td
                    className="px-4 py-1.5 font-mono text-xs text-muted-foreground truncate"
                    title={item.blockId}
                  >
                    {item.blockId.length > 12 ? `${item.blockId.slice(0, 12)}...` : item.blockId}
                  </td>
                  <td className="px-4 py-1.5">
                    {item.issue.severity === "error" ? (
                      <span className="inline-flex items-center gap-1 text-xs text-destructive">
                        <AlertTriangle className="w-3 h-3" />
                        Error
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
                        <Info className="w-3 h-3" />
                        Warning
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-1.5 text-xs text-muted-foreground">{item.issue.type}</td>
                  <td className="px-4 py-1.5 text-xs text-foreground">{item.issue.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
