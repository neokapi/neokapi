import { Button, cn } from "@neokapi/ui-primitives";
import type { TMMatchInfo, BlockTermMatch, EntityInfo } from "../../types/api";
import { ArrowRight } from "../icons";
import { entityLabel } from "./HighlightedSource";
import { tmScoreClass, termStatusClass } from "./blockStatus";

export interface ContextPanelProps {
  tmMatches: TMMatchInfo[];
  termMatches: BlockTermMatch[];
  entities?: EntityInfo[];
  loading?: boolean;
  /** Apply a TM match (by index) to the active block's target. */
  onApplyTM: (index: number) => void;
  /** Insert a target term into the active editor. */
  onInsertTerm?: (text: string) => void;
  /** Highlight a TM match as already applied (e.g. after Apply). */
  appliedTMIndex?: number | null;
  /** The active project's id — used to flag same-project vs cross-project TM. */
  currentProjectId?: string;
  /** Which sections to render. Defaults to all three. */
  sections?: { tm?: boolean; terms?: boolean; entities?: boolean };
  /** Hide each section's title row (when the embedder supplies its own). */
  hideSectionTitles?: boolean;
  className?: string;
}

/**
 * ContextPanel renders the per-block linguistic context — TM matches,
 * terminology, and entities — in a single reusable surface. It is the one
 * source of truth shared by the Translate editor's Visual card and the Review
 * surface, replacing the formerly-duplicated TM/term/entity blocks.
 */
export function ContextPanel({
  tmMatches,
  termMatches,
  entities = [],
  loading,
  onApplyTM,
  onInsertTerm,
  appliedTMIndex = null,
  currentProjectId,
  sections,
  hideSectionTitles = false,
  className,
}: ContextPanelProps) {
  const showTM = sections?.tm ?? true;
  const showTerms = sections?.terms ?? true;
  const showEntities = sections?.entities ?? true;

  return (
    <div className={cn("overflow-auto p-3", className)} data-testid="context-panel">
      {loading && <div className="text-center py-3 text-xs text-muted-foreground">Loading...</div>}

      {/* TM Matches */}
      {showTM && (
        <div className="mb-4">
          {!hideSectionTitles && (
            <div className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2 pb-1 border-b border-border">
              TM Matches
              {tmMatches.length > 0 && (
                <span className="ml-1.5 font-normal text-[10px]">({tmMatches.length})</span>
              )}
            </div>
          )}
          {!loading && tmMatches.length === 0 ? (
            <div className="text-xs text-muted-foreground italic py-2">
              No TM matches for this block
            </div>
          ) : (
            tmMatches.map((m, i) => (
              <div
                key={i}
                className={cn(
                  "p-2 bg-muted rounded-md mb-1.5 border border-border",
                  appliedTMIndex === i && "border-success bg-success/5",
                )}
                data-testid={`tm-match-${i}`}
              >
                <div className="flex justify-between mb-1">
                  <span
                    className={cn(
                      "text-[11px] font-bold px-1.5 py-px rounded",
                      tmScoreClass(m.score),
                    )}
                  >
                    {Math.round(m.score * 100)}%
                  </span>
                  <span className="text-[10px] text-muted-foreground">
                    {m.match_type.replace(/-/g, " ")}
                  </span>
                  {m.project_id && currentProjectId && (
                    <span
                      className={cn(
                        "text-[10px] px-1 py-px rounded ml-1",
                        m.project_id === currentProjectId
                          ? "text-success dark:text-success bg-success/10"
                          : "text-info dark:text-info bg-info/10",
                      )}
                    >
                      {m.project_id === currentProjectId ? "same project" : "cross-project"}
                    </span>
                  )}
                </div>
                <div className="text-xs mb-1 text-muted-foreground">{m.source}</div>
                <div className="text-xs font-medium">{m.target}</div>
                <Button
                  size="sm"
                  className={cn(
                    "mt-1.5 text-[11px] h-6 px-2",
                    appliedTMIndex === i && "bg-success hover:bg-success opacity-80 cursor-default",
                  )}
                  onClick={() => onApplyTM(i)}
                  data-testid={`tm-apply-${i}`}
                  disabled={appliedTMIndex === i}
                >
                  {appliedTMIndex === i ? "Applied" : "Apply"}
                </Button>
              </div>
            ))
          )}
        </div>
      )}

      {/* Term Matches */}
      {showTerms && (
        <div className="mb-4">
          {!hideSectionTitles && (
            <div className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2 pb-1 border-b border-border">
              Terminology
              {termMatches.length > 0 && (
                <span className="ml-1.5 font-normal text-[10px]">({termMatches.length})</span>
              )}
            </div>
          )}
          {!loading && termMatches.length === 0 ? (
            <div className="text-xs text-muted-foreground italic py-2">
              No terms found in this block
            </div>
          ) : (
            termMatches.map((m, i) => (
              <div
                key={i}
                className="p-2 bg-muted rounded-md mb-1.5 border border-border"
                data-testid={`term-match-${i}`}
              >
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
                {m.target_terms && m.target_terms.length > 0 ? (
                  <div className="flex flex-wrap items-center gap-1 mt-1">
                    <ArrowRight className="w-3 h-3 text-muted-foreground shrink-0" />
                    {m.target_terms.map((term, ti) =>
                      onInsertTerm ? (
                        <button
                          key={ti}
                          type="button"
                          className="text-xs font-medium px-1.5 py-0.5 rounded hover:bg-primary/10 hover:text-primary transition-colors cursor-pointer"
                          onClick={() => onInsertTerm(term)}
                          data-testid={`term-insert-${i}-${ti}`}
                        >
                          {term}
                        </button>
                      ) : (
                        <span key={ti} className="text-xs font-medium">
                          {term}
                          {ti < m.target_terms.length - 1 ? "," : ""}
                        </span>
                      ),
                    )}
                  </div>
                ) : (
                  <div className="text-xs italic text-muted-foreground mt-1">
                    No target term defined
                  </div>
                )}
                {m.domain && (
                  <span className="inline-block mt-1 text-[10px] text-muted-foreground px-1.5 py-px rounded bg-card border border-border">
                    {m.domain}
                  </span>
                )}
              </div>
            ))
          )}
        </div>
      )}

      {/* Entities */}
      {showEntities && (
        <div>
          {!hideSectionTitles && (
            <div className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2 pb-1 border-b border-border">
              Entities
              {entities.length > 0 && (
                <span className="ml-1.5 font-normal text-[10px]">({entities.length})</span>
              )}
            </div>
          )}
          {!loading && entities.length === 0 ? (
            <div className="text-xs text-muted-foreground italic py-2">
              No entities in this block
            </div>
          ) : (
            entities.map((e: EntityInfo, i: number) => (
              <div
                key={e.key}
                className="p-2 bg-muted rounded-md mb-1.5 border border-border"
                data-testid={`entity-${i}`}
              >
                <div className="flex items-center gap-1.5 mb-1">
                  <span className="text-[13px] font-semibold">{e.text}</span>
                  {e.dnt && (
                    <span className="text-[10px] font-semibold px-1.5 py-px rounded bg-destructive/10 text-destructive">
                      DNT
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-1.5">
                  <span className="text-[10px] px-1.5 py-px rounded bg-card border border-border text-muted-foreground">
                    {entityLabel(e.type)}
                  </span>
                  {e.source && (
                    <span className="text-[10px] text-muted-foreground">{e.source}</span>
                  )}
                </div>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
}
