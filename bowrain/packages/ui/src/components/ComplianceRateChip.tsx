import { cn, SimpleTooltip } from "@neokapi/ui-primitives";
import type { ComplianceBasis } from "../types/api";
import { ShieldCheck } from "./icons";

/**
 * ComplianceRateChip renders the per-locale compliance rate the server derives from
 * the loop's own evidence — rule-based check results plus, where the worker's draft
 * scoring has run, persisted voice scores against the profile's minimum
 * bar. The tooltip states the basis explicitly, so a checks-only rate is never
 * mistaken for a voice-informed one. Rendered only when the server sent the
 * fields (older servers omit them and the chip stays hidden).
 */
export interface ComplianceRateChipProps {
  /** compliant_blocks / translated_blocks, in [0,1]. */
  rate: number;
  /** What informed the rate (tooltip honesty line). */
  basis: ComplianceBasis;
  /** Translated blocks counting as compliant, for the tooltip detail line. */
  compliantBlocks?: number;
  /** Translated blocks in the scope, for the tooltip detail line. */
  translatedBlocks?: number;
  className?: string;
}

const basisExplanations: Record<ComplianceBasis, string> = {
  checks: "Based on rule-based checks only. No voice scores exist for this locale yet.",
  "checks+terms": "Based on rule-based checks plus terminology compliance for this locale.",
  "voice+checks":
    "Based on rule-based checks plus voice scores measured against the profile's minimum bar.",
  "voice+checks+terms":
    "Based on rule-based checks, terminology compliance, and voice scores measured against the profile's minimum bar.",
};

export function ComplianceRateChip({
  rate,
  basis,
  compliantBlocks,
  translatedBlocks,
  className,
}: ComplianceRateChipProps) {
  const pct = Math.round(Math.min(Math.max(rate, 0), 1) * 100);
  const tooltip = (
    <div className="max-w-60 space-y-1">
      <p className="font-medium">On-brand rate</p>
      <p>{basisExplanations[basis]}</p>
      {compliantBlocks !== undefined && translatedBlocks !== undefined && (
        <p className="text-muted-foreground">
          {compliantBlocks} of {translatedBlocks} translated blocks compliant
        </p>
      )}
    </div>
  );

  return (
    <SimpleTooltip content={tooltip}>
      <span
        data-testid="compliant-rate"
        data-basis={basis}
        className={cn(
          "inline-flex items-center gap-1 rounded-full border border-border/60 bg-muted px-2 py-0.5 text-xs font-medium tabular-nums text-muted-foreground",
          className,
        )}
      >
        <ShieldCheck className="h-3 w-3" />
        {pct}% compliant
      </span>
    </SimpleTooltip>
  );
}
