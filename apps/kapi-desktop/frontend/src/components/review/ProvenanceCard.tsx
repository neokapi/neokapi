import { AlertTriangle } from "lucide-react";
import { Badge, Card, CardContent } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReviewProvenance, TargetOrigin } from "../../types/api";

/**
 * Where this translation came from, and the decision in force over it.
 *
 * There is one decision per unit and variant, and a new one overwrites it, so
 * this is the decision that stands rather than the latest of a chain.
 */
export interface ProvenanceCardProps {
  provenance?: ReviewProvenance;
  /** The flat fields the unit carries, used until the model arrives. */
  origin?: TargetOrigin;
  reviewState?: string;
  note?: string;
}

export function ProvenanceCard({ provenance, origin, reviewState, note }: ProvenanceCardProps) {
  const kind = provenance?.origin?.kind ?? origin?.kind;
  const engine = provenance?.origin?.engine ?? origin?.engine;
  const tool = provenance?.origin?.tool ?? origin?.tool;
  const at = provenance?.at ?? provenance?.origin?.timestamp ?? origin?.timestamp;
  const state = provenance?.review_state ?? reviewState;
  const by = provenance?.by;
  const decisionNote = provenance?.note ?? note;
  const empty = !kind && !state && !decisionNote && !by;

  return (
    <Card data-slot="review-provenance">
      <CardContent className="space-y-1 p-3 text-xs">
        <div className="mb-1.5 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          {t("Provenance")}
          {provenance?.stale && (
            <Badge
              variant="outline"
              className="border-amber-500/40 text-[10px] normal-case text-amber-600 dark:text-amber-400"
              data-slot="review-provenance-stale"
            >
              <AlertTriangle size={10} />
              {t("decided on wording that has since changed")}
            </Badge>
          )}
        </div>

        {kind && (
          <div className="flex items-center gap-1.5">
            <span className="text-muted-foreground">{t("Origin")}</span>
            <span translate="no">
              {kind}
              {engine ? ` · ${engine}` : ""}
              {tool ? ` · ${tool}` : ""}
            </span>
          </div>
        )}

        {state && (
          <div className="flex flex-wrap items-center gap-1.5" data-slot="review-decision">
            <span className="text-muted-foreground">{t("Decision in force")}</span>
            <Badge variant="outline">{state}</Badge>
            {by && (
              <span className="text-muted-foreground">
                {t("by")} <span translate="no">{by}</span>
              </span>
            )}
            {at && <span className="text-muted-foreground tabular-nums">{at}</span>}
          </div>
        )}

        {!state && at && (
          <div className="flex items-center gap-1.5">
            <span className="text-muted-foreground">{t("Written")}</span>
            <span className="tabular-nums">{at}</span>
          </div>
        )}

        {decisionNote && (
          <div className="flex items-start gap-1.5" data-slot="review-note">
            <span className="shrink-0 text-muted-foreground">{t("Note")}</span>
            <span>{decisionNote}</span>
          </div>
        )}

        {empty && (
          <div className="text-muted-foreground">
            {t("No provenance recorded for this translation.")}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
