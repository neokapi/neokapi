import { Badge, SimpleTooltip, findingSeverityBadgeClass } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { AIReviewFinding } from "../../types/api";

/**
 * The AI pre-review's verdict on this unit, read from the state store: a score,
 * the model that produced it, and the remarks behind it.
 *
 * It sits inside the checks group because a reviewer weighs both at once, and
 * two cards saying "here is what has already been said about this translation"
 * read as two subjects.
 */
export interface AIPreReviewProps {
  score?: number;
  model?: string;
  findings?: AIReviewFinding[];
}

export function AIPreReview({ score, model, findings }: AIPreReviewProps) {
  const remarks = findings ?? [];
  if (score === undefined && remarks.length === 0) return null;

  return (
    <div className="mt-2.5 border-t pt-2" data-slot="review-ai-prereview">
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
          {t("AI pre-review")}
        </span>
        {score !== undefined && (
          <SimpleTooltip
            content={
              model
                ? t("Scored by {model}, stored with the translation it judged.", { model })
                : t("Stored with the translation it judged.")
            }
          >
            <span className="tabular-nums text-xs" data-slot="review-ai-score">
              {score}
              {model ? (
                <span className="ml-1 text-muted-foreground" translate="no">
                  {model}
                </span>
              ) : null}
            </span>
          </SimpleTooltip>
        )}
      </div>
      {remarks.length > 0 && (
        <ul className="mt-1 space-y-1">
          {remarks.map((f, i) => (
            <li key={i} className="flex items-start gap-2 text-xs" data-slot="review-ai-finding">
              <Badge variant="outline" className={findingSeverityBadgeClass(f.severity)}>
                {f.severity ?? t("note")}
              </Badge>
              <span className="min-w-0">
                <span>{f.message}</span>
                {f.suggestion && (
                  <span className="block text-muted-foreground">&#8629; {f.suggestion}</span>
                )}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
