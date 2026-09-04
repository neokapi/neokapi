import { History } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { directionAttrs } from "../../lib/text-direction";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Skeleton } from "../ui/skeleton";
import { LayerCard } from "./LayerCard";
import type { ReviewHistoryView } from "./types";

/**
 * What this unit said before, and the wording the content memory already holds
 * for it.
 *
 * A percentage on its own tells a reviewer that something close exists and
 * never what it says, so both halves carry their text. The prior version is
 * marked when its context no longer matches the one the decision was recorded
 * under, which is exactly when a translate prompt withholds it. That mark stays
 * neutral: it describes the context this unit sits in, and the severities
 * belong to the Checks card. A surface with a write offers the match's wording
 * as the target through `onUseMatch`.
 */
export interface HistoryCardProps {
  history?: ReviewHistoryView;
  sourceLocale?: string;
  locale?: string;
  loading?: boolean;
  /** The bare match percent the unit carries, shown when the model has not
   *  arrived and the wording behind it is therefore unknown. */
  fallbackMemoryScore?: number;
  /** Take the match's wording as the target. Offered only where the surface has a write. */
  onUseMatch?: () => void;
  /** The match has been taken already. */
  matchApplied?: boolean;
  /** What an empty history says. The default names only the memory, which is
   *  what every host carries; a host that also carries prior approvals passes
   *  the fuller sentence. */
  emptyText?: string;
  defaultOpen?: boolean;
  testId?: string;
  className?: string;
}

export function HistoryCard({
  history,
  sourceLocale,
  locale,
  loading,
  fallbackMemoryScore,
  onUseMatch,
  matchApplied,
  emptyText,
  defaultOpen,
  testId,
  className,
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
          : t("No close match in the content memory.");

  return (
    <LayerCard
      title={t("Already approved")}
      icon={<History size={12} className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden />}
      summary={summary}
      dataSlot="review-history"
      testId={testId}
      toggleLabel={t("What was approved for this unit before")}
      defaultOpen={defaultOpen}
      className={className}
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
          <div className="space-y-1" data-slot="review-history-match" data-testid="memory-match">
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-muted-foreground">{t("Content memory best match")}</span>
              <span
                className="tabular-nums"
                data-slot="review-memory-score"
                data-testid="memory-match-score"
                title={t("{score}% match against the source of this unit", {
                  score: match.score,
                })}
              >
                {match.score}%
              </span>
              {match.kind && (
                <span className="text-[10px] text-muted-foreground">
                  {match.kind.replace(/-/g, " ")}
                </span>
              )}
            </div>
            <div className="rounded-md border bg-muted/30 px-2 py-1">
              {match.source && (
                <span
                  className="block whitespace-pre-wrap text-muted-foreground"
                  translate="no"
                  data-testid="memory-match-source"
                  {...directionAttrs(sourceLocale)}
                >
                  {match.source}
                </span>
              )}
              <span
                className="block whitespace-pre-wrap"
                translate="no"
                data-testid="memory-match-target"
                {...directionAttrs(locale)}
              >
                {match.target}
              </span>
            </div>
            {onUseMatch && (
              <Button
                size="sm"
                variant="outline"
                className="mt-1 h-6 px-2 text-[11px]"
                onClick={onUseMatch}
                disabled={matchApplied}
                data-slot="review-memory-use"
                data-testid="memory-match-use"
              >
                {matchApplied ? t("Used") : t("Use this wording")}
              </Button>
            )}
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
              : (emptyText ?? t("No content-memory match for this block."))}
          </p>
        )}
      </div>
    </LayerCard>
  );
}
