import { Card, CardContent, Skeleton, directionAttrs } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReviewNeighbour, ReviewNeighbourhood } from "../../types/api";
import { RunText } from "./RunText";

/**
 * The unit in its document: the blocks before it, the unit, and the blocks
 * after it, in the order the file holds them.
 *
 * This is one of the four things a translate prompt carries about a block, so a
 * reviewer reading a paragraph in sequence reads what the model read. The
 * neighbours travel as run sequences and are drawn through the declared run
 * projection, chips and all.
 */

function NeighbourRow({
  neighbour,
  sourceLocale,
  locale,
}: {
  neighbour: ReviewNeighbour;
  sourceLocale?: string;
  locale?: string;
}) {
  return (
    <li className="flex gap-2 px-2 py-1" data-slot="review-neighbour">
      <span
        className="w-24 shrink-0 truncate font-mono text-[10px] text-muted-foreground"
        translate="no"
      >
        {neighbour.key}
      </span>
      <span className="min-w-0 flex-1 space-y-0.5">
        <RunText
          runs={neighbour.source}
          className="block text-muted-foreground"
          dirAttrs={directionAttrs(sourceLocale)}
        />
        {neighbour.target && neighbour.target.length > 0 && (
          <RunText runs={neighbour.target} className="block" dirAttrs={directionAttrs(locale)} />
        )}
      </span>
    </li>
  );
}

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

  return (
    <Card data-slot="review-neighbourhood">
      <CardContent className="p-3 text-xs">
        <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          {t("In the document")}
        </div>
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
          <ul className="divide-y divide-border/60 rounded-md border">
            {before.map((n, i) => (
              <NeighbourRow
                key={`before-${n.key ?? i}`}
                neighbour={n}
                sourceLocale={sourceLocale}
                locale={locale}
              />
            ))}
            <li className="flex gap-2 bg-primary/5 px-2 py-1.5" data-slot="review-neighbour-unit">
              <span
                className="w-24 shrink-0 break-all font-mono text-[10px] font-medium"
                translate="no"
              >
                {key}
              </span>
              <span className="min-w-0 flex-1 space-y-0.5">
                <span
                  className="block whitespace-pre-wrap text-muted-foreground"
                  translate="no"
                  {...directionAttrs(sourceLocale)}
                >
                  {unitSource}
                </span>
                {unitTarget ? (
                  <span
                    className="block whitespace-pre-wrap font-medium"
                    translate="no"
                    {...directionAttrs(locale)}
                  >
                    {unitTarget}
                  </span>
                ) : null}
              </span>
            </li>
            {after.map((n, i) => (
              <NeighbourRow
                key={`after-${n.key ?? i}`}
                neighbour={n}
                sourceLocale={sourceLocale}
                locale={locale}
              />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
