import { cn, resolveLocaleName, SimpleTooltip } from "@neokapi/ui-primitives";
import type { LocaleTranslationStats } from "../../types/api";

import { ComplianceRateChip } from "../ComplianceRateChip";
import { ShipStateBadge } from "../ShipStateBadge";

/**
 * LocaleCoverageRail states one locale's standing in one scope as a single
 * three-part rail: approved, translated but not yet approved, and untranslated.
 *
 * A lone percentage cannot separate the two states that decide whether a scope
 * ships — a fully translated locale awaiting review reads identically to a
 * governed one. The rail's three segments carry the colours the ship states
 * already use elsewhere (approved reads as governed, translated as
 * AI-shippable, the remainder as the empty track), so the badge beside it names
 * what the rail shows rather than adding a second, unrelated scale.
 *
 * It is the same component at one locale and at twenty: one row, sized by its
 * container, dense enough to stack into a ladder.
 */
export interface LocaleCoverageRailProps {
  stats: LocaleTranslationStats;
  /** Show the ship state badge (hidden where the server derived none). */
  showShipState?: boolean;
  /** Show the compliance rate chip where the server derived one. */
  showCompliance?: boolean;
  className?: string;
}

function pct(part: number, whole: number): number {
  if (whole <= 0) return 0;
  return Math.min(100, Math.max(0, (part / whole) * 100));
}

export function LocaleCoverageRail({
  stats,
  showShipState = true,
  showCompliance = true,
  className,
}: LocaleCoverageRailProps) {
  const total = stats.total_blocks;
  // A producer that derives no review decisions omits approved_blocks; the rail
  // then shows the whole translated run as awaiting review, which is what an
  // unknown review state honestly is.
  const approved = Math.min(stats.approved_blocks ?? 0, stats.translated_blocks);
  // Approved is a subset of translated, so the two segments are laid side by
  // side rather than stacked: approved first, then the translated remainder.
  const approvedPct = pct(approved, total);
  const translatedPct = pct(stats.translated_blocks, total);
  const pendingPct = Math.max(0, translatedPct - approvedPct);
  const coverage = Math.round(stats.percentage);
  const name = stats.display_name ?? resolveLocaleName(stats.locale, undefined, "short");

  const tooltip = (
    <div className="max-w-60 space-y-1">
      <p className="font-medium">{name}</p>
      <p>
        {stats.translated_blocks} of {total} blocks translated
      </p>
      <p className="text-muted-foreground">{approved} human-approved</p>
    </div>
  );

  return (
    <div
      className={cn("flex items-center gap-2 text-xs", className)}
      data-testid={`locale-rail-${stats.locale}`}
    >
      {/* Fixed width so the rails align into a ladder; the full name stays
          reachable as a title where a long one is cut. */}
      <span className="w-28 shrink-0 truncate text-muted-foreground" title={name}>
        {name}
      </span>
      <SimpleTooltip content={tooltip}>
        <div
          className="flex h-2 min-w-0 flex-1 overflow-hidden rounded-full bg-muted"
          role="img"
          aria-label={`${name}: ${coverage}% translated, ${approved} of ${total} blocks approved`}
        >
          <div className="h-full bg-success" style={{ width: `${approvedPct}%` }} />
          <div className="h-full bg-info" style={{ width: `${pendingPct}%` }} />
        </div>
      </SimpleTooltip>
      <span className="w-9 shrink-0 text-right tabular-nums text-muted-foreground">
        {coverage}%
      </span>
      {showCompliance && stats.compliance_rate !== undefined && stats.compliance_basis && (
        <ComplianceRateChip
          rate={stats.compliance_rate}
          basis={stats.compliance_basis}
          compliantBlocks={stats.compliant_blocks}
          translatedBlocks={stats.translated_blocks}
        />
      )}
      {showShipState && stats.ship_state && (
        <ShipStateBadge
          compact
          state={stats.ship_state}
          approvedBlocks={stats.approved_blocks}
          totalBlocks={stats.total_blocks}
          failingChecks={stats.failing_checks}
        />
      )}
    </div>
  );
}

/**
 * A scope's locales as a ladder of rails. The list scrolls inside its own box
 * so a project translating into twenty languages neither truncates its tail nor
 * pushes the card's actions off the grid.
 */
export function LocaleCoverageRails({
  locales,
  className,
  ...railProps
}: { locales: LocaleTranslationStats[]; className?: string } & Omit<
  LocaleCoverageRailProps,
  "stats" | "className"
>) {
  if (locales.length === 0) return null;
  return (
    <div className={cn("max-h-40 space-y-1.5 overflow-y-auto pr-1", className)}>
      {locales.map((l) => (
        <LocaleCoverageRail key={l.locale} stats={l} {...railProps} />
      ))}
    </div>
  );
}
