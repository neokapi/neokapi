import { cn, SimpleTooltip } from "@neokapi/ui-primitives";
import type { OnBrandBasis } from "../types/api";
import { ShieldCheck } from "./icons";

/**
 * OnBrandRateChip renders the per-locale on-brand rate the server derives from
 * the loop's own evidence — QA check results plus, where the worker's draft
 * scoring has run, persisted brand voice scores against the profile's minimum
 * bar. The tooltip states the basis explicitly, so a checks-only rate is never
 * mistaken for a voice-informed one. Rendered only when the server sent the
 * fields (older servers omit them and the chip stays hidden).
 */
export interface OnBrandRateChipProps {
  /** on_brand_blocks / translated_blocks, in [0,1]. */
  rate: number;
  /** What informed the rate (tooltip honesty line). */
  basis: OnBrandBasis;
  /** Translated blocks counting as on-brand, for the tooltip detail line. */
  onBrandBlocks?: number;
  /** Translated blocks in the scope, for the tooltip detail line. */
  translatedBlocks?: number;
  className?: string;
}

const basisExplanations: Record<OnBrandBasis, string> = {
  checks: "Based on QA checks only — no brand voice scores exist for this locale yet.",
  "checks+terms": "Based on QA checks plus terminology compliance for this locale.",
  "voice+checks":
    "Based on QA checks plus brand voice scores measured against the profile's minimum bar.",
  "voice+checks+terms":
    "Based on QA checks, terminology compliance, and brand voice scores measured against the profile's minimum bar.",
};

export function OnBrandRateChip({
  rate,
  basis,
  onBrandBlocks,
  translatedBlocks,
  className,
}: OnBrandRateChipProps) {
  const pct = Math.round(Math.min(Math.max(rate, 0), 1) * 100);
  const tooltip = (
    <div className="max-w-60 space-y-1">
      <p className="font-medium">On-brand rate</p>
      <p>{basisExplanations[basis]}</p>
      {onBrandBlocks !== undefined && translatedBlocks !== undefined && (
        <p className="text-muted-foreground">
          {onBrandBlocks} of {translatedBlocks} translated blocks on-brand
        </p>
      )}
    </div>
  );

  return (
    <SimpleTooltip content={tooltip}>
      <span
        data-testid="on-brand-rate"
        data-basis={basis}
        className={cn(
          "inline-flex items-center gap-1 rounded-full border border-border/60 bg-muted px-2 py-0.5 text-xs font-medium tabular-nums text-muted-foreground",
          className,
        )}
      >
        <ShieldCheck className="h-3 w-3" />
        {pct}% on-brand
      </span>
    </SimpleTooltip>
  );
}
