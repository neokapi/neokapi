// A term's status as a word on its tint, and the words for a status and a
// relation in the reader's language. The tints are the concept UI's, so a term
// reads the same colour here as on Kapi Desktop's Terms page.
import { t } from "@neokapi/i18n-react/runtime";
import { TERM_STATUS_CLASS, type RelationType, type TermStatus } from "@neokapi/concept-ui";
import { Badge, cn } from "@neokapi/ui-primitives";

/** A term status in the active UI language. */
export function termStatusLabel(status: TermStatus): string {
  switch (status) {
    case "preferred":
      return t("Preferred", "term status");
    case "approved":
      return t("Approved", "term status");
    case "admitted":
      return t("Admitted", "term status");
    case "proposed":
      return t("Proposed", "term status");
    case "deprecated":
      return t("Deprecated", "term status");
    case "forbidden":
      return t("Forbidden", "term status");
    default:
      return status;
  }
}

/** A term's status, by its word, on the tint the concept UI gives it. */
export function TermStatusChip({ status, className }: { status: TermStatus; className?: string }) {
  return (
    <Badge
      variant="outline"
      data-slot="term-status"
      data-status={status}
      className={cn("h-4 px-1.5 text-[11px]", TERM_STATUS_CLASS[status], className)}
    >
      {termStatusLabel(status)}
    </Badge>
  );
}

/** A relation kind, read from the selected concept, in the active UI language. */
export function relationWord(type: RelationType): string {
  switch (type) {
    case "BROADER":
      return t("broader than", "concept relation");
    case "NARROWER":
      return t("narrower than", "concept relation");
    case "PART_OF":
      return t("part of", "concept relation");
    case "HAS_PART":
      return t("has part", "concept relation");
    case "RELATED":
      return t("related to", "concept relation");
    case "REPLACED_BY":
      return t("replaced by", "concept relation");
    case "USE_INSTEAD":
      return t("use instead", "concept relation");
    case "EXACT_MATCH":
      return t("exact match", "concept relation");
    case "CLOSE_MATCH":
      return t("close match", "concept relation");
    case "COMPETITOR":
      return t("competitor", "concept relation");
    default:
      return type;
  }
}
