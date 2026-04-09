import { useState, useCallback, useMemo } from "react";
import type { TMGroupedResult, TMTargetDTO } from "./types";
import type { SpanInfo } from "../../types/span";
import { CodedTextDisplay } from "./CodedTextDisplay";
import { InlineCodeEditor } from "../editor/InlineCodeEditor";
import { LocalePill } from "./LocalePill";
import { ItemCard } from "../ui/item-card";
import { Checkbox } from "../ui/checkbox";
import { Button } from "../ui/button";
import { ConfirmDeleteButton } from "../ui/confirm-delete-button";
import { relativeTime } from "./utils";
import { ChevronRight } from "lucide-react";
import { cn } from "../../lib/utils";

const AUTO_EXPAND_THRESHOLD = 10;

interface TMGroupedEntryProps {
  group: TMGroupedResult;
  selected: boolean;
  onToggleSelect: () => void;
  onEditTarget: (targetId: string, codedText: string, spans: SpanInfo[]) => void;
  onDeleteTarget: (targetId: string) => void;
  /**
   * Filter visible targets by locale. `undefined` = show all.
   * An array (even empty) = show only those locales — empty means hide all.
   */
  visibleLocales?: string[];
}

/**
 * Card for a source text with all its target translations.
 * Auto-expands when fewer than 10 targets; otherwise collapsible.
 * Supports filtering visible targets by locale.
 */
export function TMGroupedEntry({
  group,
  selected,
  onToggleSelect,
  onEditTarget,
  onDeleteTarget,
  visibleLocales,
}: TMGroupedEntryProps) {
  const autoExpand = group.targets.length < AUTO_EXPAND_THRESHOLD;
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(null);
  const expanded = manualExpanded ?? autoExpand;
  const [editingTargetId, setEditingTargetId] = useState<string | null>(null);

  const filteredTargets = useMemo(() => {
    if (visibleLocales === undefined) return group.targets;
    return group.targets.filter((t) => visibleLocales.includes(t.target_locale));
  }, [group.targets, visibleLocales]);

  const handleSave = useCallback(
    (target: TMTargetDTO, codedText: string, spans: SpanInfo[]) => {
      onEditTarget(target.id, codedText, spans);
      setEditingTargetId(null);
    },
    [onEditTarget],
  );

  const hiddenCount = group.targets.length - filteredTargets.length;

  return (
    <ItemCard selected={selected} className="p-3" data-testid={`tm-group-${group.source_text.slice(0, 20)}`}>
      <div className="flex items-start gap-2">
        <Checkbox
          checked={selected}
          onCheckedChange={onToggleSelect}
          className="mt-1 shrink-0"
          aria-label={`Select group ${group.source_text}`}
        />

        <div className="flex-1 min-w-0">
          {/* Source header */}
          <button
            className="flex items-start gap-2 w-full text-left"
            onClick={() => setManualExpanded(expanded ? false : true)}
          >
            {!autoExpand && (
              <ChevronRight
                className={cn(
                  "size-4 shrink-0 mt-0.5 text-muted-foreground transition-transform",
                  expanded && "rotate-90",
                )}
              />
            )}
            <LocalePill locale={group.source_locale} />
            <CodedTextDisplay
              text={group.source_text}
              codedText={group.source_coded}
              spans={group.source_spans}
              className="text-[14px] font-medium text-foreground flex-1"
            />
            <span className="text-[10px] text-muted-foreground bg-muted px-1.5 py-px rounded tabular-nums shrink-0">
              {filteredTargets.length}{hiddenCount > 0 ? `/${group.targets.length}` : ""} {group.targets.length === 1 ? "translation" : "translations"}
            </span>
          </button>

          {/* Target translations */}
          {expanded && filteredTargets.length > 0 && (
            <div className={cn("mt-1.5 flex flex-col gap-1", !autoExpand && "ml-6 border-l-2 border-border/50 pl-3")}>
              {filteredTargets.map((target) => (
                <div key={target.id} className="group/target flex items-start gap-2">
                  {editingTargetId === target.id ? (
                    <div className="flex-1">
                      <InlineCodeEditor
                        initialCodedText={target.target_coded || target.target_text}
                        initialSpans={target.target_spans || []}
                        sourceSpans={group.source_spans || []}
                        onSave={(codedText, spans) => handleSave(target, codedText, spans)}
                        onCancel={() => setEditingTargetId(null)}
                        compact
                      />
                    </div>
                  ) : (
                    <>
                      <LocalePill locale={target.target_locale} />
                      <CodedTextDisplay
                        text={target.target_text}
                        codedText={target.target_coded}
                        spans={target.target_spans}
                        className="text-[13px] text-muted-foreground flex-1"
                      />
                      <span className="text-[10px] text-muted-foreground shrink-0">
                        {relativeTime(target.updated_at)}
                      </span>
                      <div className="flex gap-1 opacity-0 transition-opacity group-hover/target:opacity-100 shrink-0">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-5 px-1 text-[10px] text-muted-foreground"
                          onClick={() => setEditingTargetId(target.id)}
                        >
                          Edit
                        </Button>
                        <ConfirmDeleteButton
                          onDelete={() => onDeleteTarget(target.id)}
                          mode="inline"
                        />
                      </div>
                    </>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </ItemCard>
  );
}
