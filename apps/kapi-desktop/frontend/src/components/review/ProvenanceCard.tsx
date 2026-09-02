import { AlertTriangle, Fingerprint } from "lucide-react";
import { Badge } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReviewProvenance, TargetOrigin } from "../../types/api";
import { LayerCard } from "./LayerCard";

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

/**
 * How an origin kind reads to a person. The platform's provenance card names
 * the same kinds the same way, so a reviewer working across both reads one
 * vocabulary. A kind with no phrase here is shown as the model records it.
 */
function originLabel(kind: string): string | undefined {
  switch (kind) {
    case "human":
      return t("Written by a person");
    case "memory":
      return t("Recycled from content memory");
    case "mt":
      return t("Machine translation");
    case "ai":
      return t("AI translation");
    case "ocr":
      return t("Read from an image");
    case "asr":
      return t("Transcribed from audio");
    default:
      return undefined;
  }
}

export function ProvenanceCard({ provenance, origin, reviewState, note }: ProvenanceCardProps) {
  const kind = provenance?.origin?.kind ?? origin?.kind;
  const engine = provenance?.origin?.engine ?? origin?.engine;
  const tool = provenance?.origin?.tool ?? origin?.tool;
  const producer = [engine, tool].filter(Boolean).join(" · ");
  const label = kind ? originLabel(kind) : undefined;
  const at = provenance?.at ?? provenance?.origin?.timestamp ?? origin?.timestamp;
  const state = provenance?.review_state ?? reviewState;
  const by = provenance?.by;
  const decisionNote = provenance?.note ?? note;
  const empty = !kind && !state && !decisionNote && !by;

  const summary = (
    <>
      <span className="text-foreground">
        {label ?? (kind ? kind : t("No provenance recorded."))}
      </span>
      {state && <Badge variant="outline">{state}</Badge>}
      {provenance?.stale && (
        <Badge
          variant="outline"
          className="border-warning/40 text-[10px] text-warning"
          data-slot="review-provenance-stale"
        >
          <AlertTriangle size={10} />
          {t("decided on wording that has since changed")}
        </Badge>
      )}
    </>
  );

  return (
    <LayerCard
      title={t("Provenance")}
      icon={<Fingerprint size={12} className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden />}
      summary={summary}
      dataSlot="review-provenance"
      toggleLabel={t("Where this translation came from")}
    >
      <div className="space-y-1">
        {kind && (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-muted-foreground">{t("Origin")}</span>
            {label ? <span>{label}</span> : <span translate="no">{kind}</span>}
            {producer && (
              <span className="text-muted-foreground" translate="no">
                {producer}
              </span>
            )}
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
      </div>
    </LayerCard>
  );
}
