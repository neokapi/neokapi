import { Layers } from "lucide-react";
import { NeighbourhoodTable, Skeleton } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReviewNeighbourhood } from "../../types/api";
import { LayerCard } from "./LayerCard";

/**
 * The unit in its document: the blocks before it, the unit, and the blocks
 * after it, in the order the file holds them.
 *
 * This is one of the four things a translate prompt carries about a block, so a
 * reviewer reading a paragraph in sequence reads what the model read. The table
 * itself is `NeighbourhoodTable` in the shared kit, which the platform's review
 * surfaces draw too.
 */

export interface NeighbourhoodCardProps {
  neighbourhood?: ReviewNeighbourhood;
  /** The unit under decision, rendered in place between its neighbours. */
  unitKey?: string;
  unitSource?: string;
  unitTarget?: string;
  sourceLocale?: string;
  locale?: string;
  loading?: boolean;
}

export function NeighbourhoodCard({
  neighbourhood,
  unitKey,
  unitSource,
  unitTarget,
  sourceLocale,
  locale,
  loading,
}: NeighbourhoodCardProps) {
  const before = neighbourhood?.before ?? [];
  const after = neighbourhood?.after ?? [];
  const key = neighbourhood?.key ?? unitKey ?? "";
  const around = before.length + after.length;
  const summary = !neighbourhood
    ? loading
      ? t("Reading the document…")
      : t("The blocks around this unit could not be read.")
    : around === 0
      ? t("This unit stands alone in its document.")
      : t("{before} before, {after} after", { before: before.length, after: after.length });

  return (
    <LayerCard
      title={t("In the document")}
      icon={<Layers size={12} className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden />}
      summary={summary}
      dataSlot="review-neighbourhood"
      toggleLabel={t("The blocks around this unit")}
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
              {t("The blocks around this unit could not be read.")}
            </p>
          )
        ) : (
          <NeighbourhoodTable
            before={before}
            after={after}
            unitKey={key}
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
