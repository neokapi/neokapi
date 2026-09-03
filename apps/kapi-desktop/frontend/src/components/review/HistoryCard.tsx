import { History } from "lucide-react";
import { Badge, Skeleton, directionAttrs } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReviewHistory } from "../../types/api";
import { LayerCard } from "./LayerCard";

/**
 * What this unit said before, and the wording the content memory already holds
 * for it.
 *
 * A percentage on its own tells a reviewer that something close exists and
 * never what it says, so both halves carry their text. The prior version is
 * marked when its fingerprint no longer matches the context the decision was
 * recorded under, which is exactly when a translate prompt withholds it. That
 * mark stays neutral: it describes the context this unit sits in, and the
 * severities belong to the Checks card.
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
  const score = match?.score ?? bareScore;

  // The verdict, in the order a reviewer weighs it: what this unit said before,
  // then what the content memory holds for something like it.
  const summary = prior
    ? prior.governed
      ? t("Approved before, still governed")
      : t("Approved before, under a context that has moved")
    : score !== undefined
      ? t("Content memory best match {score}%", { score })
      : !history && loading
        ? t("Reading what was approved before…")
        : history?.unseeded
          ? t("The committed content memory has not been read into this copy yet.")
          : t("Nothing approved yet, and no close match.");

  return (
    <LayerCard
      title={t("Already approved")}
      icon={<History size={12} className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden />}
      summary={summary}
      dataSlot="review-history"
      toggleLabel={t("What was approved for this unit before")}
    >
      <div className="space-y-2">
        {!history && loading && <Skeleton className="h-4 w-3/5" />}

        {prior && (
          <div className="space-y-1" data-slot="review-history-prior">
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-muted-foreground">{t("Previous version")}</span>
              {prior.governed ? (
                <Badge variant="outline" className="text-[11px] text-muted-foreground">
                  {t("still governed")}
                </Badge>
              ) : (
                <Badge
                  variant="outline"
                  className="text-[11px] text-muted-foreground"
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
          <p className="text-muted-foreground" data-slot="review-history-empty">
            {history.unseeded
              ? t(
                  "The committed content memory has not been read into this copy of the project yet, so nothing can be matched. Bring up to date reads it.",
                )
              : t(
                  "Nothing has been approved for this unit yet, and the content memory holds no close match.",
                )}
          </p>
        )}
      </div>
    </LayerCard>
  );
}
