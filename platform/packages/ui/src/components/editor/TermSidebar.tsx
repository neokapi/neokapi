import { ArrowRight, Plus, BookOpen } from "lucide-react";
import { BlockTermMatch } from "../../types/api";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { cn } from "../../lib/utils";
import type { VisualEditorMode } from "./visual-editor-types";

export interface TermSidebarProps {
  termMatches: BlockTermMatch[];
  loading?: boolean;
  onInsertTerm: (text: string) => void;
  onAddTerm?: () => void;
  editorMode?: VisualEditorMode;
}

function termStatusClass(status: string): string {
  const colors: Record<string, string> = {
    preferred: "text-green-500 bg-green-500/[0.08]",
    approved: "text-blue-500 bg-blue-500/[0.08]",
    admitted: "text-orange-600 bg-orange-600/[0.08]",
    deprecated: "text-red-500 bg-red-500/[0.08]",
  };
  return colors[status] || "text-muted-foreground bg-muted";
}

export function TermSidebar({
  termMatches,
  loading,
  onInsertTerm,
  onAddTerm,
  editorMode,
}: TermSidebarProps) {
  return (
    <div
      className="w-[260px] min-w-[260px] border-l border-border bg-card overflow-y-auto p-3 shrink-0 glass-surface"
      data-testid="term-sidebar"
    >
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-1.5">
          <BookOpen className="w-3.5 h-3.5 text-muted-foreground" />
          <span className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider">
            Terminology
          </span>
          {termMatches.length > 0 && (
            <Badge variant="secondary" className="text-[10px] px-1.5 py-0 h-4 ml-0.5">
              {termMatches.length}
            </Badge>
          )}
        </div>
        {editorMode === "enrich" && onAddTerm && (
          <Button
            size="sm"
            variant="ghost"
            className="h-6 w-6 p-0"
            onClick={onAddTerm}
            data-testid="term-add-btn"
          >
            <Plus className="w-3.5 h-3.5" />
          </Button>
        )}
      </div>

      {/* Loading state */}
      {loading && (
        <div className="flex items-center justify-center py-6">
          <div className="h-4 w-4 rounded-full border-2 border-muted-foreground/30 border-t-primary animate-spin" />
        </div>
      )}

      {/* Empty state */}
      {!loading && termMatches.length === 0 && (
        <div className="text-xs text-muted-foreground italic py-4 text-center">
          No terminology matches
        </div>
      )}

      {/* Term list */}
      {!loading &&
        termMatches.map((m, i) => (
          <div
            key={i}
            className="p-2 bg-muted rounded-md mb-1.5 border border-border"
            data-testid={`term-match-${i}`}
          >
            {/* Source term + status */}
            <div className="flex items-center gap-1.5 mb-1">
              <span className="text-[13px] font-semibold">{m.source_term}</span>
              <span
                className={cn(
                  "text-[10px] font-semibold px-1.5 py-px rounded",
                  termStatusClass(m.status),
                )}
              >
                {m.status}
              </span>
            </div>

            {/* Target translations */}
            {m.target_terms && m.target_terms.length > 0 ? (
              <div className="flex flex-wrap items-center gap-1 mt-1">
                <ArrowRight className="w-3 h-3 text-muted-foreground shrink-0" />
                {m.target_terms.map((term, ti) => (
                  <button
                    key={ti}
                    type="button"
                    className="text-xs font-medium px-1.5 py-0.5 rounded hover:bg-primary/10 hover:text-primary transition-colors cursor-pointer"
                    onClick={() => onInsertTerm(term)}
                    data-testid={`term-insert-${i}-${ti}`}
                  >
                    {term}
                  </button>
                ))}
              </div>
            ) : (
              <div className="text-xs italic text-muted-foreground mt-1">
                No target term defined
              </div>
            )}

            {/* Domain badge */}
            {m.domain && (
              <span className="inline-block mt-1.5 text-[10px] text-muted-foreground px-1.5 py-px rounded bg-card border border-border">
                {m.domain}
              </span>
            )}
            {m.project_id && (
              <span className="inline-block mt-0.5 text-[10px] px-1 py-px rounded bg-blue-500/10 text-blue-600 dark:text-blue-400 ml-1">
                project
              </span>
            )}
          </div>
        ))}
    </div>
  );
}
