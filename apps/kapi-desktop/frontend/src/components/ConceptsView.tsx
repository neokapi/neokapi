import { useMemo, useState } from "react";
import {
  ConceptList,
  ConceptDashboard,
  type ConceptDataSource,
  type ConceptViewSlots,
} from "@neokapi/concept-ui";
import { createLocalConceptSource } from "../lib/localConceptSource";

/**
 * The sections Kapi Desktop does NOT render.
 *
 * Discussion (threaded comments) is an *interaction* surface: it presupposes
 * other people, a shared record, and someone to answer. That belongs to the
 * platform — Bowrain owns review conversations, and its REST concept source is
 * the only one that can supply them. Observations are the same shape of thing
 * from the other direction: server-gathered external evidence.
 *
 * Kapi Desktop reads and edits a *local* termbase, so neither is available to it
 * (see `localConceptSource.ts` — it supplies no markets/observations/comments/
 * timeline/whereUsed). Leaving the slots wired rendered two permanently-empty
 * panels inviting a conversation the app cannot hold.
 *
 * Withholding the slots — rather than forking the shared dashboard — is the
 * supported extension point: `ConceptDashboard` spreads `defaultConceptSlots`
 * under the caller's, and `ConceptView` gates each region on its slot being
 * present. Bowrain keeps the full set from the same component.
 */
const DESKTOP_SLOTS: ConceptViewSlots = {
  comments: undefined,
  observations: undefined,
};

export interface ConceptsViewProps {
  /** The open termbase handle this view is bound to. */
  handle: string;
  /**
   * Pre-built source for Storybook/tests (skips the Wails backend). When omitted
   * a local source is created for `handle`.
   */
  source?: ConceptDataSource;
  /** Scope concept terms to these locales (the project's Active Filter). */
  localeScope?: string[];
}

/**
 * The visual concept/relation workspace over a LOCAL termbase. The list is the
 * entry surface; opening a concept shows its dashboard (terms, relations,
 * tag-derived geography, constraints, synthesized timeline); a relation
 * re-centres the dashboard on its neighbour; Back returns to the list. Relations
 * and term statuses are edited inline — this is the desktop home for the editing
 * the deleted CLI relation commands used to do.
 */
export function ConceptsView({ handle, source: injected, localeScope }: ConceptsViewProps) {
  const source = useMemo(() => injected ?? createLocalConceptSource(handle), [injected, handle]);
  const [openId, setOpenId] = useState<string | null>(null);

  if (openId) {
    return (
      <ConceptDashboard
        conceptId={openId}
        source={source}
        slots={DESKTOP_SLOTS}
        localeScope={localeScope}
        onNavigate={setOpenId}
        onBack={() => setOpenId(null)}
      />
    );
  }
  return <ConceptList source={source} localeScope={localeScope} onOpen={setOpenId} />;
}
