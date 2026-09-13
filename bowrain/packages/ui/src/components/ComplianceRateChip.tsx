import { cn, SimpleTooltip } from "@neokapi/ui-primitives";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import type { ComplianceBasis } from "../types/api";
import { ShieldCheck } from "./icons";

/**
 * ComplianceRateChip renders the per-locale compliance rate the server derives from
 * the loop's own evidence: rule-based check results, terminology where terms or
 * voice profile rules govern the language, and the voice bar where a voice
 * profile does. The tooltip names what governs the language, so a checks-only
 * rate is never mistaken for a voice-informed one.
 *
 * The rate is over blocks with a verdict. A block with no result for a governing
 * dimension is not checked, and a block in a language nothing beyond the checks
 * governs is not governed. Neither counts toward the rate, and the chip names
 * how many of each there are beside it; with no block holding a verdict there is
 * no rate, and the chip shows the counts alone. Rendered only when the server
 * sent the fields (older servers omit them and the chip stays hidden).
 */
export interface ComplianceRateChipProps {
  /** compliant_blocks over the translated blocks with a verdict, in [0,1]; absent when none has one. */
  rate?: number;
  /** What governs the language (tooltip honesty line). */
  basis: ComplianceBasis;
  /** Translated blocks counting as compliant, for the tooltip detail line. */
  compliantBlocks?: number;
  /** Translated blocks in the scope, for the tooltip detail line. */
  translatedBlocks?: number;
  /** Translated blocks with no result for a governing dimension, which the rate leaves out. */
  notCheckedBlocks?: number;
  /** Translated blocks in a language nothing beyond the checks governs, which the rate leaves out. */
  notGovernedBlocks?: number;
  className?: string;
}

const basisExplanations: Record<ComplianceBasis, string> = {
  checks: "Only the rule-based checks apply. No terms and no voice profile apply to this language.",
  "checks+terms":
    "Based on rule-based checks plus terminology compliance. No voice profile applies to this language.",
  "voice+checks":
    "Based on rule-based checks plus voice scores measured against the profile's minimum bar. No terms apply to this language.",
  "voice+checks+terms":
    "Based on rule-based checks, terminology compliance, and voice scores measured against the profile's minimum bar.",
};

export function ComplianceRateChip({
  rate,
  basis,
  compliantBlocks,
  translatedBlocks,
  notCheckedBlocks,
  notGovernedBlocks,
  className,
}: ComplianceRateChipProps) {
  const notChecked = notCheckedBlocks ?? 0;
  const notGoverned = notGovernedBlocks ?? 0;
  const pct = rate === undefined ? undefined : Math.round(Math.min(Math.max(rate, 0), 1) * 100);
  const judgedBlocks =
    translatedBlocks === undefined ? undefined : translatedBlocks - notChecked - notGoverned;
  const tooltip = (
    <div className="max-w-60 space-y-1">
      <p className="font-medium">On-brand rate</p>
      <p>{basisExplanations[basis]}</p>
      {pct !== undefined && compliantBlocks !== undefined && judgedBlocks !== undefined && (
        <p className="text-muted-foreground">
          {compliantBlocks} of {judgedBlocks} judged blocks compliant
        </p>
      )}
      {notChecked > 0 && (
        <p className="text-muted-foreground" data-testid="compliance-not-checked-detail">
          <Plural count={notChecked}>
            <One>
              {notChecked} translated block has no result for a check that applies to this language:
              its terminology had nothing to check, or nothing has scored it against the voice
              profile. It counts toward neither side of the rate.
            </One>
            <Other>
              {notChecked} translated blocks have no result for a check that applies to this
              language: their terminology had nothing to check, or nothing has scored them against
              the voice profile. They count toward neither side of the rate.
            </Other>
          </Plural>
        </p>
      )}
      {notGoverned > 0 && (
        <p className="text-muted-foreground" data-testid="compliance-not-governed-detail">
          <Plural count={notGoverned}>
            <One>
              {notGoverned} translated block is in a language no terms and no voice profile apply
              to, so only the rule-based checks judged it. It counts toward neither side of the
              rate.
            </One>
            <Other>
              {notGoverned} translated blocks are in a language no terms and no voice profile apply
              to, so only the rule-based checks judged them. They count toward neither side of the
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
        {(pct !== undefined || notChecked > 0) && notGoverned > 0 && (
          <span aria-hidden="true">·</span>
        )}
        {notGoverned > 0 && (
          <span data-testid="compliance-not-governed">{notGoverned} not governed</span>
        )}
      </span>
    </SimpleTooltip>
  );
}
