// ConceptDashboard — the per-concept view fully composed (Apache-2.0). This is
// the surface a consuming app renders: it wires the production section panels
// into the ConceptView shell, so kapi-desktop (local termbase) and bowrain
// (REST) both get the identical dashboard from just a data source. Sections
// gate themselves on capabilities, so the local-termbase-only path degrades to
// terms, relations, geography (from tags), constraints, and the synthesized
// timeline — observations and discussion appear only when the source supplies
// them.
import { ConceptView, type ConceptViewProps, type ConceptViewSlots } from "./ConceptView";
import { RelationsPanel } from "./RelationsPanel";
import { MarketsPanel } from "./MarketsPanel";
import { ConceptTimeline } from "./ConceptTimeline";
import { ConstraintsPanel } from "./ConstraintsPanel";
import { ObservationsPanel } from "./ObservationsPanel";
import { CommentsPanel } from "./CommentsPanel";

/**
 * The production section renderers, ready to drop into `ConceptView`. Exported
 * so a consumer can override one slot while keeping the rest, e.g.
 * `slots={{ ...defaultConceptSlots, relations: myRelations }}`.
 */
export const defaultConceptSlots: Required<
  Pick<
    ConceptViewSlots,
    "geography" | "relations" | "timeline" | "constraints" | "observations" | "comments"
  >
> = {
  geography: (p) => <MarketsPanel {...p} />,
  relations: (p) => <RelationsPanel {...p} />,
  timeline: (p) => <ConceptTimeline {...p} />,
  constraints: (p) => <ConstraintsPanel {...p} />,
  observations: (p) => <ObservationsPanel {...p} />,
  comments: (p) => <CommentsPanel {...p} />,
};

export type ConceptDashboardProps = Omit<ConceptViewProps, "slots"> & {
  /** Override or extend the default section renderers. */
  slots?: ConceptViewSlots;
};

export function ConceptDashboard({ slots, ...rest }: ConceptDashboardProps) {
  return <ConceptView {...rest} slots={{ ...defaultConceptSlots, ...slots }} />;
}
