import { Layers } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { NeighbourhoodTable, type NeighbourhoodEntry } from "../ui/neighbourhood-table";
import { Skeleton } from "../ui/skeleton";
import { LayerCard } from "./LayerCard";
import type { ReviewNeighbourhood } from "@neokapi/contract-types";

/**
 * The unit in its document: the blocks before it, the unit, and the blocks
 * after it, in the order the file holds them.
 *
 * This is one of the four things a translate prompt carries about a block, so a
 * reviewer reading a paragraph in sequence reads what the model read. The table
 * is `NeighbourhoodTable`, and the neighbours travel as run sequences through
 * the declared run projection, so a placeholder in a neighbour, or in the unit
 * itself, reads as a chip rather than disappearing.
 */
export interface NeighbourhoodCardProps {
  neighbourhood?: ReviewNeighbourhood;
  /** The unit under decision, rendered in place between its neighbours. */
  unitKey?: string;
  /**
   * The unit's own runs, as the caller holds them now. Wins over the
   * neighbourhood's copy, which was read with the context and can trail an
   * edit made since.
   */
  unit?: NeighbourhoodEntry;
  /** The unit's text, drawn only when no runs for it are at hand. */
  unitSource?: string;
  unitTarget?: string;
  sourceLocale?: string;
  locale?: string;
  loading?: boolean;
  defaultOpen?: boolean;
  testId?: string;
  className?: string;
}

export function NeighbourhoodCard({
  neighbourhood,
  unitKey,
  unit,
  unitSource,
  unitTarget,
  sourceLocale,
  locale,
  loading,
  defaultOpen,
  testId,
  className,
}: NeighbourhoodCardProps) {
  const before = neighbourhood?.before ?? [];
  const after = neighbourhood?.after ?? [];
  const key = neighbourhood?.key ?? unitKey ?? "";
  const around = before.length + after.length;
  const summary = !neighbourhood
    ? loading
      ? t("Reading the document…")
      : t("The blocks around this one could not be read.")
    : around === 0
      ? t("This block stands alone in its document.")
      : t("{before} before, {after} after", { before: before.length, after: after.length });

  return (
    <LayerCard
      title={t("In the document")}
      icon={<Layers size={12} className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden />}
      summary={summary}
      dataSlot="review-neighbourhood"
      testId={testId}
      toggleLabel={t("The blocks around this one")}
      defaultOpen={defaultOpen}
      className={className}
    >
      <div className="text-xs">
        {!neighbourhood ? (
          loading ? (
            <div className="space-y-1.5">
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-4/5" />
            </div>
          ) : (
            <p className="text-muted-foreground">
              {t("The blocks around this one could not be read.")}
            </p>
          )
        ) : (
          <NeighbourhoodTable
            before={before}
            after={after}
            unitKey={key}
            unit={unit ?? neighbourhood.unit}
            unitSource={unitSource}
            unitTarget={unitTarget}
            sourceLocale={sourceLocale}
            targetLocale={locale}
          />
        )}
      </div>
    </LayerCard>
  );
}
