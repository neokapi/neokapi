import { useState } from "react";
import { ChevronRight } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button, Collapsible, CollapsibleContent, TermRuleChip } from "@neokapi/ui-primitives";
import type { ReviewPoint } from "@neokapi/contract-types";

/** How many term rules show before the rest go behind "N more". The host lists
 *  the rules bearing on this block's wording first. */
const TERMS_SHOWN = 4;

export interface ReviewPointSummaryProps {
  point?: ReviewPoint;
  className?: string;
}

/**
 * What a reviewer uses from the point a block sits at: the voice profile in
 * force and the term rules that apply to this block. Coordinates, store ids
 * and resolution notes stay out of the review pane; the voice's rendered
 * guidance sits behind a disclosure.
 *
 * Draws nothing until the point arrives, or when it carries neither a voice nor
 * a term rule.
 */
export function ReviewPointSummary({ point, className }: ReviewPointSummaryProps) {
  const [termsOpen, setTermsOpen] = useState(false);
  const [guideOpen, setGuideOpen] = useState(false);
  if (!point) return null;
  const voice = point.voice;
  const rules = point.term_rules ?? [];
  if (!voice && rules.length === 0) return null;
  const shown = termsOpen ? rules : rules.slice(0, TERMS_SHOWN);
  const hidden = rules.length - shown.length;

  return (
    <div className={className} data-slot="review-point">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
        {voice && (
          <span className="flex items-center gap-1.5">
            <span className="text-muted-foreground">{t("Voice")}</span>
            <span className="font-medium" translate="no" data-slot="review-point-voice">
              {voice.name}
            </span>
            {voice.guide && (
              <Button
                variant="ghost"
                size="xs"
                className="h-5 gap-0.5 px-1 text-[11px] text-muted-foreground"
                onClick={() => setGuideOpen((v) => !v)}
                aria-expanded={guideOpen}
                data-slot="review-point-guide-toggle"
              >
                <ChevronRight
                  size={12}
                  className={guideOpen ? "rotate-90 transition-transform" : "transition-transform"}
                />
                {t("Guidance")}
              </Button>
            )}
          </span>
        )}
        {rules.length > 0 && (
          <span className="flex flex-wrap items-center gap-1" data-slot="review-point-terms">
            <span className="mr-0.5 text-muted-foreground">{t("Terms")}</span>
            {shown.map((rule, i) => (
              <TermRuleChip key={`${rule.term}-${i}`} rule={rule} index={i} />
            ))}
            {hidden > 0 && (
              <Button
                variant="ghost"
                size="xs"
                className="h-5 px-1 text-[11px] text-muted-foreground"
                onClick={() => setTermsOpen(true)}
                data-slot="review-point-terms-more"
              >
                {t("{count} more", { count: hidden })}
              </Button>
            )}
          </span>
        )}
      </div>
      {voice?.guide && (
        <Collapsible open={guideOpen}>
          <CollapsibleContent>
            <pre
              className="mt-2 max-h-56 overflow-auto whitespace-pre-wrap rounded-md bg-muted/40 p-2 text-[11px] leading-snug"
              data-slot="review-point-guide"
            >
              {voice.guide}
            </pre>
          </CollapsibleContent>
        </Collapsible>
      )}
    </div>
  );
}
