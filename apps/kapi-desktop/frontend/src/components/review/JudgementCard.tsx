import { CheckCircle2, ShieldCheck } from "lucide-react";
import { Badge, findingSeverityBadgeClass } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { AIReviewFinding, DesktopFinding } from "../../types/api";
import { AIPreReview } from "./AIPreReview";
import { LayerCard } from "./LayerCard";

/**
 * What has already been said about this translation: the checks that ran over
 * it, and the AI pre-review that scored it.
 *
 * The summary is the verdict a reviewer acts on, so it counts the findings and
 * names the worst severity among them rather than saying that findings exist.
 */

/** The severity ordering the summary reads, worst first. */
const SEVERITY_RANK = ["critical", "major", "minor", "neutral"];

/** The worst severity in a set of findings, or undefined for a clean unit. */
function worstSeverity(findings: DesktopFinding[]): string | undefined {
  let worst: string | undefined;
  for (const f of findings) {
    const s = (f.severity ?? "").toLowerCase();
    const rank = SEVERITY_RANK.indexOf(s);
    if (rank < 0) continue;
    if (worst === undefined || rank < SEVERITY_RANK.indexOf(worst)) worst = s;
  }
  return worst;
}

export interface JudgementCardProps {
  /** The unit's check findings, or undefined while the unit is loading. */
  findings?: DesktopFinding[];
  /** The AI pre-review's stored score and the model that produced it. */
  aiScore?: number;
  aiModel?: string;
  aiFindings?: AIReviewFinding[];
}

export function JudgementCard({ findings, aiScore, aiModel, aiFindings }: JudgementCardProps) {
  const list = findings ?? [];
  const worst = worstSeverity(list);

  const summary = (
    <>
      {list.length === 0 ? (
        <span className="flex items-center gap-1.5">
          <CheckCircle2 size={12} className="text-success" aria-hidden />
          {t("No findings")}
        </span>
      ) : (
        <>
          <span className="text-foreground">{t("{count} findings", { count: list.length })}</span>
          {worst && (
            <Badge variant="outline" className={findingSeverityBadgeClass(worst)}>
              {worst}
            </Badge>
          )}
        </>
      )}
      {aiScore !== undefined && (
        <span className="tabular-nums" data-slot="review-judgement-score">
          {t("AI {score}", { score: aiScore })}
        </span>
      )}
    </>
  );

  return (
    <LayerCard
      title={t("Checks")}
      icon={<ShieldCheck size={12} className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden />}
      summary={summary}
      dataSlot="review-findings"
      toggleLabel={t("What the checks found")}
    >
      {list.length === 0 ? (
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <CheckCircle2 size={12} className="text-success" />
          {t("No findings for this unit.")}
        </div>
      ) : (
        <ul className="space-y-1.5">
          {list.map((f: DesktopFinding, i: number) => (
            <li key={i} className="flex items-start gap-2 text-xs" data-slot="review-finding">
              <Badge variant="outline" className={findingSeverityBadgeClass(f.severity)}>
                {f.severity}
              </Badge>
              {/* Which side of the unit the finding is about. A voice or
                  terminology finding is judged on the SOURCE, and unlabelled it
                  reads as a defect in the translation: a reviewer saw
                  `Forbidden term "cart"` on a Norwegian target containing no
                  such word, with nothing to act on and no way to tell why. */}
              {f.field === "source" && (
                <Badge
                  variant="outline"
                  className="shrink-0 text-[10px] text-muted-foreground"
                  title={t("This finding is about the source text, not the translation.")}
                >
                  {t("source")}
                </Badge>
              )}
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
      <AIPreReview score={aiScore} model={aiModel} findings={aiFindings} />
    </LayerCard>
  );
}
