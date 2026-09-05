import { AlertTriangle, Fingerprint } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Badge } from "../ui/badge";
import { When } from "../ui/when";
import { LayerCard } from "./LayerCard";
import type { ReviewProvenance } from "@neokapi/contract-types";

/**
 * Where this translation came from, and the decision in force over it.
 *
 * There is one decision per unit and variant, and a new one overwrites it, so
 * this is the decision that stands rather than the latest of a chain. Both
 * surfaces name the origin kinds and the decision states the same way, so a
 * reviewer working across them reads one vocabulary.
 */

/** How an origin kind reads to a person. A kind with no phrase is shown as recorded. */
export function originLabel(kind: string | undefined): string | undefined {
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

/** How a decision's review state reads to a person. */
export function decisionLabel(state: string | undefined): string | undefined {
  switch (state) {
    case "approved":
      return t("Approved");
    case "rejected":
      return t("Rejected");
    case "signed-off":
      return t("Signed off");
    default:
      return state || undefined;
  }
}

export interface ProvenanceCardProps {
  provenance?: ReviewProvenance;
  /**
   * A note on the unit outside the decision record, where the host keeps
   * one: the platform's latest block note. The decision's own note wins.
   */
  note?: string;
  defaultOpen?: boolean;
  testId?: string;
  className?: string;
}

export function ProvenanceCard({
  provenance,
  note: unitNote,
  defaultOpen,
  testId,
  className,
}: ProvenanceCardProps) {
  const origin = provenance?.origin;
  const kind = origin?.kind;
  const producer = [origin?.engine, origin?.tool].filter(Boolean).join(" · ");
  const label = originLabel(kind);
  const state = decisionLabel(provenance?.review_state);
  const by = provenance?.by;
  const at = provenance?.at;
  const stale = provenance?.stale;
  const note = provenance?.note || unitNote;
  const empty = !kind && !state && !by && !note;

  const summary = (
    <>
      <span className="text-foreground">
        {label ?? kind ?? (state ? "" : t("Nothing recorded"))}
      </span>
      {state && <Badge variant="outline">{state}</Badge>}
      {stale && (
        <Badge
          variant="outline"
          className="h-auto whitespace-normal border-warning/40 text-left text-[11px] text-warning"
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
      testId={testId}
      toggleLabel={t("Where this translation came from")}
      defaultOpen={defaultOpen}
      className={className}
    >
      <div className="space-y-1" data-testid="review-provenance">
        {kind && (
          <div className="flex flex-wrap items-center gap-1.5" data-testid="provenance-origin">
            <span className="text-muted-foreground">{t("Origin")}</span>
            {label ? <span>{label}</span> : <span translate="no">{kind}</span>}
            {producer && (
              <span className="text-muted-foreground" translate="no">
                {producer}
              </span>
            )}
          </div>
        )}

        {(state || by) && (
          <div
            className="flex flex-wrap items-center gap-1.5"
            data-slot="review-decision"
            data-testid="provenance-decision"
          >
            <span className="text-muted-foreground">
              {state ? t("Decision in force") : t("Last change")}
            </span>
            {state && <Badge variant="outline">{state}</Badge>}
            {by && (
              <span className="text-muted-foreground">
                {t("by")} <span translate="no">{by}</span>
              </span>
            )}
            {at && <When iso={at} className="text-muted-foreground" />}
          </div>
        )}

        {stale && (
          <div className="flex items-center gap-1.5 text-warning">
            <AlertTriangle size={11} aria-hidden />
            {t("The source has changed since that decision.")}
          </div>
        )}

        {!state && !by && (at ?? origin?.timestamp) && (
          <div className="flex items-center gap-1.5">
            <span className="text-muted-foreground">{t("Written")}</span>
            <When iso={(at ?? origin?.timestamp)!} />
          </div>
        )}

        {note && (
          <div
            className="flex items-start gap-1.5"
            data-slot="review-note"
            data-testid="provenance-note"
          >
            <span className="shrink-0 text-muted-foreground">{t("Note")}</span>
            <span>{note}</span>
          </div>
        )}

        {empty && (
          <div className="text-muted-foreground">
            {t("No decision recorded, and no provenance stamped.")}
          </div>
        )}
      </div>
    </LayerCard>
  );
}
