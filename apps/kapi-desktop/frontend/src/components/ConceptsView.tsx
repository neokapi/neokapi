import { useMemo, useState } from "react";
import { ConceptList, ConceptDashboard, type ConceptDataSource } from "@neokapi/concept-ui";
import { createLocalConceptSource } from "../lib/localConceptSource";

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
        localeScope={localeScope}
        onNavigate={setOpenId}
        onBack={() => setOpenId(null)}
      />
    );
  }
  return <ConceptList source={source} localeScope={localeScope} onOpen={setOpenId} />;
}
