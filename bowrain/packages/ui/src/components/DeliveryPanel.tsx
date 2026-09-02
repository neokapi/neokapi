import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  cn,
  LocaleLabel,
  SimpleTooltip,
} from "@neokapi/ui-primitives";
import type { LocaleTranslationStats } from "../types/api";
import { AlertTriangle, ArrowRight, Clock, Plug } from "./icons";

import { ShipStateBadge } from "./ShipStateBadge";

/**
 * DeliveryPanel is the read-only "what can I ship, and where did it go" card:
 * per-locale ship state plus each connector's last delivery (last sync time
 * and most recent sync failure). It renders data the caller already fetched —
 * the dashboard stats' locale slice and the connector status adapter results —
 * and only navigates via the provided callbacks.
 */

/** One connector's delivery status, from the connector status adapter call. */
export interface DeliveryConnectorStatus {
  id: string;
  /** Display name (caller falls back to the connector id when unnamed). */
  name: string;
  /** RFC3339 time of the last successful fetch/publish; empty = never synced. */
  lastSync?: string;
  /** Most recent sync failure, cleared by the next successful sync. */
  lastError?: string;
}

export interface DeliveryPanelProps {
  /** Per-locale stats (with ship states) from the translation dashboard. */
  localeStats: LocaleTranslationStats[];
  /** Connector delivery statuses; undefined while loading, [] when none. */
  connectors?: DeliveryConnectorStatus[];
  /** Navigate to the connectors panel. */
  onOpenConnectors?: () => void;
  /** Navigate to the review surface; shown while any locale awaits review. */
  onOpenReview?: () => void;
  className?: string;
}

function formatLastSync(lastSync?: string): string {
  const t = lastSync ? new Date(lastSync) : null;
  const valid = t && !Number.isNaN(t.getTime()) && t.getTime() > 0;
  return valid ? `Synced ${t.toLocaleString()}` : "Never synced";
}

/**
 * A locale awaits review while some translated block has no review decision.
 * Requires ship-state data: legacy servers without it cannot say either way,
 * so the review link stays hidden rather than guessing.
 */
function pendingReview(ls: LocaleTranslationStats): boolean {
  return ls.ship_state !== undefined && ls.translated_blocks > (ls.approved_blocks ?? 0);
}

export function DeliveryPanel({
  localeStats,
  connectors,
  onOpenConnectors,
  onOpenReview,
  className,
}: DeliveryPanelProps) {
  const anyPendingReview = localeStats.some(pendingReview);

  return (
    <Card data-testid="delivery-panel" className={cn("gap-3", className)}>
      <CardHeader className="pb-0">
        <CardTitle className="text-sm">Delivery</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Per-locale ship states */}
        <ul className="space-y-1.5">
          {localeStats.map((ls) => {
            const pct = Math.round(ls.percentage);
            return (
              <li key={ls.locale} className="flex items-center justify-between gap-2 text-sm">
                <LocaleLabel locale={ls.locale} displayName={ls.display_name} hideCode />
                {ls.ship_state ? (
                  <ShipStateBadge
                    state={ls.ship_state}
                    approvedBlocks={ls.approved_blocks}
                    totalBlocks={ls.total_blocks}
                    failingChecks={ls.failing_checks}
                  />
                ) : (
                  <span className="text-muted-foreground text-xs">{pct}%</span>
                )}
              </li>
            );
          })}
          {localeStats.length === 0 && (
            <li className="text-muted-foreground text-xs">No target languages configured.</li>
          )}
        </ul>

        {/* Delivered where/when */}
        <div className="border-border/60 space-y-1.5 border-t pt-3">
          {(connectors ?? []).map((c) => (
            <div key={c.id} className="flex items-start gap-2 text-xs">
              <Plug className="text-muted-foreground mt-0.5 size-3.5 shrink-0" />
              <div className="min-w-0 flex-1">
                <span className="text-foreground block truncate font-medium">{c.name}</span>
                <span className="text-muted-foreground inline-flex items-center gap-1">
                  <Clock className="size-3" />
                  {formatLastSync(c.lastSync)}
                </span>
                {c.lastError && (
                  <SimpleTooltip content={c.lastError}>
                    <span className="text-destructive mt-0.5 flex items-center gap-1">
                      <AlertTriangle className="size-3 shrink-0" />
                      <span className="truncate">{c.lastError}</span>
                    </span>
                  </SimpleTooltip>
                )}
              </div>
            </div>
          ))}
          {connectors !== undefined && connectors.length === 0 && (
            <p className="text-muted-foreground text-xs">No connectors deliver this project yet.</p>
          )}
        </div>

        {/* Read-only navigation */}
        <div className="flex flex-wrap items-center gap-2">
          {onOpenConnectors && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs"
              onClick={onOpenConnectors}
              data-testid="delivery-open-connectors"
            >
              Connectors
              <ArrowRight className="size-3" />
            </Button>
          )}
          {anyPendingReview && onOpenReview && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs"
              onClick={onOpenReview}
              data-testid="delivery-open-review"
            >
              Review pending translations
              <ArrowRight className="size-3" />
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
