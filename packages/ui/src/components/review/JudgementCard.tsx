import type { ReactNode } from "react";
import { CheckCircle2, ShieldCheck } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { findingToneBadgeClass, type FindingTone } from "../../lib/finding-severity";
import { Badge } from "../ui/badge";
import { AIPreReview } from "./AIPreReview";
import { LayerCard } from "./LayerCard";
import type { ReviewAIRemarkView, ReviewFindingView } from "./types";

/**
 * What has already been said about this translation: the findings every
 * checker raised over it, as one list, and the AI pre-review that scored it.
 *
 * The summary is the verdict a reviewer acts on, so it counts the findings and
 * names the worst among them rather than saying that findings exist. A finding
 * says what it was raised against and what to say instead; one judged on the
 * source side is marked, because unlabelled it reads as a defect in the
 * translation.
 */

/** The tone ordering the summary reads, worst first. */
const TONE_RANK: FindingTone[] = ["destructive", "warning", "muted"];

/** The worst finding in a set, or undefined for a clean unit. */
function worst(findings: ReviewFindingView[]): ReviewFindingView | undefined {
  let top: ReviewFindingView | undefined;
  for (const f of findings) {
    if (!top || TONE_RANK.indexOf(f.tone) < TONE_RANK.indexOf(top.tone)) top = f;
  }
  return top;
}

export interface JudgementCardProps {
  /** The unit's findings, or undefined while the unit is loading. */
  findings?: ReviewFindingView[];
  /** The AI pre-review's stored score and the model that produced it. */
  aiScore?: number;
  aiModel?: string;
  aiFindings?: ReviewAIRemarkView[];
  /** Drawn above the list: a surface's own re-check control, or the target with the findings marked on it. */
  children?: ReactNode;
  defaultOpen?: boolean;
  testId?: string;
  className?: string;
}

export function JudgementCard({
  findings,
  aiScore,
  aiModel,
  aiFindings,
  children,
  defaultOpen,
  testId,
  className,
}: JudgementCardProps) {
  const list = findings ?? [];
  const top = worst(list);
  const failing = list.filter((f) => f.tone === "destructive").length;

  const summary = (
    <>
      {list.length === 0 ? (
        <span className="flex items-center gap-1.5">
          <CheckCircle2 size={12} className="text-success" aria-hidden />
          {t("No findings")}
        </span>
      ) : (
        <>
          <span className="text-foreground">
            {failing > 0
              ? t("{count} findings, {failing} failing", { count: list.length, failing })
              : t("{count} findings", { count: list.length })}
          </span>
          {top?.severity && (
            <Badge variant="outline" className={findingToneBadgeClass(top.tone)}>
              {top.severity}
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
      testId={testId}
      toggleLabel={t("What the checks found")}
      defaultOpen={defaultOpen}
      className={className}
    >
      {children && <div className="mb-2 space-y-2">{children}</div>}
      {list.length === 0 ? (
        <div
          className="flex items-center gap-1.5 text-xs text-muted-foreground"
          data-testid="findings-none"
        >
          <CheckCircle2 size={12} className="text-success" />
          {t("No findings for this unit.")}
        </div>
      ) : (
        <ul className="space-y-1.5" data-testid="findings">
          {list.map((f, i) => (
            <li
              key={f.id ?? i}
              className="flex items-start gap-2 text-xs"
              data-slot="review-finding"
              data-testid={f.id}
              data-tone={f.tone}
            >
              {f.severity && (
                <Badge variant="outline" className={findingToneBadgeClass(f.tone)}>
                  {f.severity}
                </Badge>
              )}
              {f.field === "source" && (
                <Badge
                  variant="outline"
                  className="shrink-0 text-[11px] text-muted-foreground"
                  title={t("This finding is about the source text, not the translation.")}
                >
                  {t("source")}
                </Badge>
              )}
              <span className="min-w-0 space-y-0.5">
                <span>
                  {f.category && <span className="font-medium">{f.category}: </span>}
                  {f.message}
                </span>
                {(f.originalText || f.suggestion) && (
                  <span className="flex flex-wrap items-center gap-1.5">
                    {f.originalText && (
                      <span
                        className="rounded bg-destructive/10 px-1.5 py-px font-mono text-[11px] text-destructive"
                        translate="no"
                        data-testid={f.id ? `${f.id}-original` : undefined}
                      >
                        {f.originalText}
                      </span>
                    )}
                    {f.suggestion && (
                      <>
                        <span className="text-muted-foreground" aria-hidden>
                          &rarr;
                        </span>
                        <span
                          className="rounded bg-success/10 px-1.5 py-px font-mono text-[11px] text-success"
                          translate="no"
                          data-testid={f.id ? `${f.id}-suggestion` : undefined}
                        >
                          {f.suggestion}
                        </span>
                      </>
                    )}
                  </span>
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
