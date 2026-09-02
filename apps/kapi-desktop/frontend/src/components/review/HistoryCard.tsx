import { Badge, Card, CardContent, Skeleton, directionAttrs } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReviewHistory } from "../../types/api";

/**
 * What this unit said before, and the wording the content memory already holds
 * for it.
 *
 * A percentage on its own tells a reviewer that something close exists and
 * never what it says, so both halves carry their text. The prior version is
 * marked when its fingerprint no longer matches the context the decision was
 * recorded under, which is exactly when a translate prompt withholds it.
 */
export interface HistoryCardProps {
  history?: ReviewHistory;
  sourceLocale?: string;
  locale?: string;
  loading?: boolean;
  /** The bare match percent the unit carries, shown when the model has not
   *  arrived and the wording behind it is therefore unknown. */
  fallbackMemoryScore?: number;
}

export function HistoryCard({
  history,
  sourceLocale,
  locale,
  loading,
  fallbackMemoryScore,
}: HistoryCardProps) {
  const prior = history?.prior;
  const match = history?.match;
  const bareScore = !match && fallbackMemoryScore ? fallbackMemoryScore : undefined;

  return (
    <Card data-slot="review-history">
      <CardContent className="space-y-2 p-3 text-xs">
        <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          {t("Already approved")}
        </div>

        {!history && loading && <Skeleton className="h-4 w-3/5" />}

        {prior && (
          <div className="space-y-1" data-slot="review-history-prior">
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-muted-foreground">{t("Previous version")}</span>
              {prior.governed ? (
                <Badge variant="outline" className="text-[10px] text-muted-foreground">
                  {t("still governed")}
                </Badge>
              ) : (
                <Badge
                  variant="outline"
                  className="border-amber-500/40 text-[10px] text-amber-600 dark:text-amber-400"
                  title={t(
                    "The governing context has moved since this was approved, so the translate prompt would not have carried it.",
                  )}
                >
                  {t("context has moved")}
                </Badge>
              )}
            </div>
            <div className="rounded-md border bg-muted/30 px-2 py-1">
              <span
                className="block whitespace-pre-wrap text-muted-foreground"
                translate="no"
                {...directionAttrs(sourceLocale)}
              >
                {prior.source}
              </span>
              <span
                className="block whitespace-pre-wrap"
                translate="no"
                {...directionAttrs(locale)}
              >
                {prior.target}
              </span>
            </div>
          </div>
        )}

        {match && (
          <div className="space-y-1" data-slot="review-history-match">
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-muted-foreground">{t("Content memory best match")}</span>
              <span className="tabular-nums" data-slot="review-memory-score">
                {match.score}%
              </span>
            </div>
            <div className="rounded-md border bg-muted/30 px-2 py-1">
              {match.source && (
                <span
                  className="block whitespace-pre-wrap text-muted-foreground"
                  translate="no"
                  {...directionAttrs(sourceLocale)}
                >
                  {match.source}
                </span>
              )}
              <span
                className="block whitespace-pre-wrap"
                translate="no"
                {...directionAttrs(locale)}
              >
                {match.target}
              </span>
            </div>
          </div>
        )}

        {bareScore !== undefined && (
          <div className="flex flex-wrap items-center gap-1.5" data-slot="review-memory-score">
            <span className="text-muted-foreground">{t("Content memory best match")}</span>
            <span className="tabular-nums">{bareScore}%</span>
          </div>
        )}

        {history && !prior && !match && bareScore === undefined && (
          <p className="text-muted-foreground">
            {t(
              "Nothing has been approved for this unit yet, and the content memory holds no close match.",
            )}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
