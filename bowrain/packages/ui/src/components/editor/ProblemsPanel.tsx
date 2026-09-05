import { useState, useMemo } from "react";
import { FindingSnippet, checkIssueTone } from "@neokapi/ui-primitives";
import type { Run } from "@neokapi/contract-types";
import type { BlockInfo, FileCheckResult, CheckIssue } from "../../types/api";
import { X, Check, AlertTriangle, Info, FileText } from "../icons";
import { getTargetText } from "./blockStatus";

interface ProblemsPanelProps {
  issues: FileCheckResult[];
  loading?: boolean;
  onNavigateToBlock: (blockId: string) => void;
  onClose: () => void;
  /**
   * The file's blocks, so each issue can be read in the text it was raised on
   * with its span marked. Absent, an issue shows the text the checker quoted.
   */
  blocks?: BlockInfo[];
  /** The locale the checks ran against, whose text is read beside the source. */
  targetLocale?: string;
  /** The project's source language, for the source text's direction. */
  sourceLocale?: string;
}

type FilterMode = "all" | "errors";

interface FlatIssue {
  blockId: string;
  issue: CheckIssue;
}

/** A block's source runs: the typed runs the server ships, else its text as one run. */
function sourceRunsOf(block: BlockInfo): Run[] {
  return block.source_runs ?? [{ text: block.source }];
}

/** A block's runs in a locale, else its text there as one run; undefined when it has none. */
function targetRunsOf(block: BlockInfo, locale: string | undefined): Run[] | undefined {
  if (!locale) return undefined;
  const runs = block.targets_runs?.[locale];
  if (runs && runs.length > 0) return runs;
  const text = getTargetText(block, locale);
  return text ? [{ text }] : undefined;
}

/**
 * ProblemsPanel slides up from the bottom to display findings,
 * similar to VS Code's problems panel. Each row reads the issue in the text it
 * was raised on and opens that block in the document.
 */
export function ProblemsPanel({
  issues,
  loading,
  onNavigateToBlock,
  onClose,
  blocks,
  targetLocale,
  sourceLocale,
}: ProblemsPanelProps) {
  const [filter, setFilter] = useState<FilterMode>("all");

  const blocksById = useMemo(() => {
    const m = new Map<string, BlockInfo>();
    for (const b of blocks ?? []) m.set(b.id, b);
    return m;
  }, [blocks]);

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
              <Info className="w-3 h-3 text-warning ml-1" />
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
            <span className="ml-2 text-sm text-muted-foreground">Running checks...</span>
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
                <th className="px-4 py-1.5 font-medium w-[1%]">
                  <span className="sr-only">Open</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((item, i) => {
                const block = blocksById.get(item.blockId);
                const blockLabel = block?.name || item.blockId;
                const tone = checkIssueTone(item.issue.severity);
                const targetRuns = block ? targetRunsOf(block, targetLocale) : undefined;
                return (
                  <tr
                    key={`${item.blockId}-${i}`}
                    onClick={() => onNavigateToBlock(item.blockId)}
                    className="border-b border-border/20 cursor-pointer hover:bg-muted/30 transition-colors"
                    data-testid="problem-row"
                  >
                    <td
                      className="px-4 py-1.5 font-mono text-xs text-muted-foreground truncate align-top"
                      title={blockLabel}
                    >
                      {blockLabel.length > 12 ? `${blockLabel.slice(0, 12)}...` : blockLabel}
                    </td>
                    <td className="px-4 py-1.5 align-top">
                      {item.issue.severity === "error" ? (
                        <span className="inline-flex items-center gap-1 text-xs text-destructive">
                          <AlertTriangle className="w-3 h-3" />
                          Error
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-xs text-warning dark:text-warning">
                          <Info className="w-3 h-3" />
                          Warning
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-1.5 text-xs text-muted-foreground align-top">
                      {item.issue.type}
                    </td>
                    <td className="px-4 py-1.5 text-xs text-foreground align-top">
                      <div>{item.issue.message}</div>
                      {/* The issue in the text it was raised on. A check's position
                          is anchored to the source runs (core/check), so the
                          source carries the mark and the checked locale's text
                          reads beneath it. */}
                      {(block || item.issue.original_text) && (
                        <div className="mt-1 space-y-0.5 text-[11px]" data-testid="problem-context">
                          <FindingSnippet
                            runs={block ? sourceRunsOf(block) : undefined}
                            locale={sourceLocale}
                            anchor={item.issue.position}
                            tone={tone}
                            label={item.issue.message}
                            fallbackText={item.issue.original_text}
                            data-testid="problem-snippet"
                          />
                          {targetRuns && (
                            <div className="text-muted-foreground" data-testid="problem-target">
                              <FindingSnippet
                                runs={targetRuns}
                                locale={targetLocale}
                                tone={tone}
                                label={item.issue.message}
                              />
                            </div>
                          )}
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-1.5 align-top">
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          onNavigateToBlock(item.blockId);
                        }}
                        className="inline-flex items-center gap-1 whitespace-nowrap rounded-md px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-muted/50 hover:text-foreground transition-colors"
                        data-testid="problem-open-document"
                      >
                        <FileText className="w-3 h-3" />
                        Open in document
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
