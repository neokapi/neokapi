import { cn, SimpleTooltip } from "@neokapi/ui-primitives";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import type { ComplianceBasis } from "../types/api";
import { ShieldCheck } from "./icons";

/**
 * ComplianceRateChip renders the per-locale compliance rate the server derives from
 * the loop's own evidence — rule-based check results plus, where the worker's draft
 * scoring has run, persisted voice scores against the profile's minimum
 * bar. The tooltip states the basis explicitly, so a checks-only rate is never
 * mistaken for a voice-informed one.
 *
 * The rate is over checked blocks only. A block whose terminology was not
 * checked counts toward neither side of it, and the chip names how many there
 * are beside the rate; with no block checked there is no rate, and the chip
 * shows that count alone. Rendered only when the server sent the fields (older
 * servers omit them and the chip stays hidden).
 */
export interface ComplianceRateChipProps {
  /** compliant_blocks over the checked translated blocks, in [0,1]; absent when none was checked. */
  rate?: number;
  /** What informed the rate (tooltip honesty line). */
  basis: ComplianceBasis;
  /** Translated blocks counting as compliant, for the tooltip detail line. */
  compliantBlocks?: number;
  /** Translated blocks in the scope, for the tooltip detail line. */
  translatedBlocks?: number;
  /** Translated blocks whose terminology was not checked, which the rate leaves out. */
  notCheckedBlocks?: number;
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
  notCheckedBlocks,
  className,
}: ComplianceRateChipProps) {
  const notChecked = notCheckedBlocks ?? 0;
  const pct = rate === undefined ? undefined : Math.round(Math.min(Math.max(rate, 0), 1) * 100);
  const checkedBlocks = translatedBlocks === undefined ? undefined : translatedBlocks - notChecked;
  const tooltip = (
    <div className="max-w-60 space-y-1">
      <p className="font-medium">On-brand rate</p>
      <p>{basisExplanations[basis]}</p>
      {pct !== undefined && compliantBlocks !== undefined && checkedBlocks !== undefined && (
        <p className="text-muted-foreground">
          {compliantBlocks} of {checkedBlocks} checked blocks compliant
        </p>
      )}
      {notChecked > 0 && (
        <p className="text-muted-foreground" data-testid="compliance-not-checked-detail">
          <Plural count={notChecked}>
            <One>
              {notChecked} translated block was not checked for terminology, because no terms or
              voice profile rules apply to this language. It counts toward neither side of the rate.
            </One>
            <Other>
              {notChecked} translated blocks were not checked for terminology, because no terms or
              voice profile rules apply to this language. They count toward neither side of the
              rate.
            </Other>
          </Plural>
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
        {pct !== undefined && <span>{pct}% compliant</span>}
        {pct !== undefined && notChecked > 0 && <span aria-hidden="true">·</span>}
        {notChecked > 0 && (
          <span data-testid="compliance-not-checked">{notChecked} not checked</span>
        )}
      </span>
    </SimpleTooltip>
  );
}
