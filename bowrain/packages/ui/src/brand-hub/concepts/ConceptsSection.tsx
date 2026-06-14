// ConceptsSection — the Concepts list section of the Brand hub (AD-021), now the
// framework concept UI. It frames @neokapi/concept-ui's ConceptList in the
// BrandHub page shell and drives it with a RestConceptDataSource built over the
// workspace's ApiAdapter (the SAME source the per-concept dashboard uses). The
// former List/Graph-toggle table and the whole-graph view are gone — the graph
// is a local widget inside one concept's dashboard, not a global canvas.
import { useMemo } from "react";
import type { ConceptDataSource } from "@neokapi/concept-ui";
import { ConceptList } from "@neokapi/concept-ui";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import { BrandHub } from "../shell/BrandHub";
import { createRestConceptSource } from "./restConceptSource";

export interface ConceptsSectionProps {
  /** Open a concept's dashboard. */
  onOpenConcept: (conceptId: string) => void;
}

export function ConceptsSection({ onOpenConcept }: ConceptsSectionProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const source: ConceptDataSource = useMemo(() => createRestConceptSource(api, ws), [api, ws]);

  return (
    <BrandHub
      title="Concepts"
      description="The language-neutral units of your brand — each with its terms, status by locale, and direct relations."
      width="wide"
    >
      <ConceptList source={source} onOpen={onOpenConcept} />
    </BrandHub>
  );
}
