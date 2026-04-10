import { useState, useCallback, useMemo } from "react";
import type { TMEntryDTO, VariantDTO } from "./types";
import type { SpanInfo } from "../../types/span";
import { CodedTextDisplay } from "./CodedTextDisplay";
import { InlineCodeEditor } from "../editor/InlineCodeEditor";
import { LocalePill } from "./LocalePill";
import { OriginsPopover } from "./OriginsPopover";
import { ItemCard } from "../ui/item-card";
import { Checkbox } from "../ui/checkbox";
import { Button } from "../ui/button";
import { ConfirmDeleteButton } from "../ui/confirm-delete-button";
import { relativeTime } from "./utils";
import { ChevronRight } from "lucide-react";
import { cn } from "../../lib/utils";

const AUTO_EXPAND_THRESHOLD = 10;

interface TMGroupedEntryProps {
  entry: TMEntryDTO;
  selected: boolean;
  onToggleSelect: () => void;
  /** Called when a variant is edited inline. The first arg is the edited locale. */
  onEditVariant: (locale: string, codedText: string, spans: SpanInfo[]) => void;
  onDelete: () => void;
  /**
   * Filter visible variants by locale. `undefined` = show all.
   * An array (even empty) = show only those locales — empty means show none.
   * The source variant (driven by `hint_src_lang`) is always shown as the header.
   */
  visibleLocales?: string[];
}

/**
 * Card for a single multilingual TM entry. The source variant (selected via
 * `entry.hint_src_lang`) is shown as the card header and every other variant
 * is rendered below. Auto-expands when the entry has fewer than 10 other
 * variants; otherwise collapsible.
 */
export function TMGroupedEntry({
  entry,
  selected,
  onToggleSelect,
  onEditVariant,
  onDelete,
  visibleLocales,
}: TMGroupedEntryProps) {
  const locales = useMemo(() => Object.keys(entry.variants), [entry.variants]);

  // Resolve the "source" variant: honour hint_src_lang if present, otherwise
  // fall back to the first locale in insertion order.
  const sourceLocale =
    entry.hint_src_lang && entry.variants[entry.hint_src_lang]
      ? entry.hint_src_lang
      : (locales[0] ?? "");
  const sourceVariant: VariantDTO | undefined = sourceLocale
    ? entry.variants[sourceLocale]
    : undefined;

  const otherVariants = useMemo(() => {
    return locales
      .filter((l) => l !== sourceLocale)
      .map((l) => entry.variants[l])
      .filter((v): v is VariantDTO => Boolean(v));
  }, [locales, entry.variants, sourceLocale]);

  const filteredVariants = useMemo(() => {
    if (visibleLocales === undefined) return otherVariants;
    return otherVariants.filter((v) => visibleLocales.includes(v.locale));
  }, [otherVariants, visibleLocales]);

  const autoExpand = otherVariants.length < AUTO_EXPAND_THRESHOLD;
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(null);
  const expanded = manualExpanded ?? autoExpand;
  const [editingLocale, setEditingLocale] = useState<string | null>(null);

  const handleSave = useCallback(
    (variant: VariantDTO, codedText: string, spans: SpanInfo[]) => {
      onEditVariant(variant.locale, codedText, spans);
      setEditingLocale(null);
    },
    [onEditVariant],
  );

  const hiddenCount = otherVariants.length - filteredVariants.length;
  const sourceText = sourceVariant?.text ?? "";
  const sourceCoded = sourceVariant?.coded ?? "";
  const sourceSpans = sourceVariant?.spans ?? [];

  const countLabel = `${filteredVariants.length}${hiddenCount > 0 ? `/${otherVariants.length}` : ""} ${otherVariants.length === 1 ? "translation" : "translations"}`;

  return (
    <ItemCard selected={selected} className="p-3" data-testid={`tm-entry-${entry.id}`}>
      <div className="flex items-start gap-2">
        <Checkbox
          checked={selected}
          onCheckedChange={onToggleSelect}
          className="mt-1 shrink-0"
          aria-label={`Select entry ${sourceText}`}
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
            <LocalePill locale={sourceLocale} />
            <CodedTextDisplay
              text={sourceText}
              codedText={sourceCoded}
              spans={sourceSpans}
              className="text-[14px] font-medium text-foreground flex-1"
            />
            <span className="text-[10px] text-muted-foreground bg-muted px-1.5 py-px rounded tabular-nums shrink-0">
              {countLabel}
            </span>
          </button>

          {/* Other variants */}
          {expanded && filteredVariants.length > 0 && (
            <div
              className={cn(
                "mt-1.5 flex flex-col gap-1",
                !autoExpand && "ml-6 border-l-2 border-border/50 pl-3",
              )}
            >
              {filteredVariants.map((variant) => (
                <div key={variant.locale} className="group/target flex items-start gap-2">
                  {editingLocale === variant.locale ? (
                    <div className="flex-1">
                      <InlineCodeEditor
                        initialCodedText={variant.coded || variant.text}
                        initialSpans={variant.spans || []}
                        sourceSpans={sourceSpans}
                        onSave={(codedText, spans) => handleSave(variant, codedText, spans)}
                        onCancel={() => setEditingLocale(null)}
                        compact
                      />
                    </div>
                  ) : (
                    <>
                      <LocalePill locale={variant.locale} />
                      <CodedTextDisplay
                        text={variant.text}
                        codedText={variant.coded}
                        spans={variant.spans}
                        className="text-[13px] text-muted-foreground flex-1"
                      />
                      <span className="text-[10px] text-muted-foreground shrink-0">
                        {relativeTime(entry.updated_at)}
                      </span>
                      <div className="flex gap-1 opacity-0 transition-opacity group-hover/target:opacity-100 shrink-0">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-5 px-1 text-[10px] text-muted-foreground"
                          onClick={() => setEditingLocale(variant.locale)}
                        >
                          Edit
                        </Button>
                      </div>
                    </>
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Footer: provenance + delete */}
          <div className="flex items-center gap-2 mt-1.5 text-[10px] text-muted-foreground">
            {entry.project_id && (
              <span className="inline-flex shrink-0 items-center px-1.5 py-px rounded font-mono text-[10px] font-medium bg-blue-500/10 text-blue-600 dark:text-blue-400">
                Project
              </span>
            )}
            <OriginsPopover origins={entry.origins ?? []} note={entry.note} />
            <div className="ml-auto flex gap-1 opacity-0 transition-opacity group-hover:opacity-100">
              <ConfirmDeleteButton onDelete={onDelete} mode="inline" />
            </div>
          </div>
        </div>
      </div>
    </ItemCard>
  );
}
