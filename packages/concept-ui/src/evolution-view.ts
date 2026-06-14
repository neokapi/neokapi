// The shared prop contract the two evolution renderers implement (Apache-2.0),
// so the responsive orchestrator (`ConceptEvolution`) can swap the horizontal
// `EvolutionRoadmap` and the vertical `EvolutionGraph` behind one type.

import type { EvolutionModel } from "./evolution-types";

/** Newest-first ("desc") or oldest-first ("asc"). */
export type EvolutionOrder = "asc" | "desc";

/** The props both `EvolutionRoadmap` and `EvolutionGraph` accept. */
export interface EvolutionViewProps {
  /** The fully-derived model (see `buildEvolutionModel`). */
  model: EvolutionModel;
  /** Display order for time-sorted content. Default "desc". */
  order?: EvolutionOrder;
  /** Re-centre the whole concept view on another concept (rename/relation). */
  onNavigate?: (conceptId: string) => void;
  className?: string;
}
