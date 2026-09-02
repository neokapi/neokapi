// ConceptsSection — the Concepts list section of the Brand hub (AD-021), now the
// framework concept UI. It frames @neokapi/concept-ui's ConceptList in the
// ContextHub page shell and drives it with a RestConceptDataSource built over the
// workspace's ApiAdapter (the SAME source the per-concept dashboard uses). The
// former List/Graph-toggle table and the whole-graph view are gone — the graph
// is a local widget inside one concept's dashboard, not a global canvas.
//
// Governed changes never appear in the list itself: a proposed concept is not a
// concept until its change-set merges. So the section carries the summons above
// the list (PendingConceptChanges) and, when the vocabulary is genuinely empty,
// says which of the two empty pages this is — nothing here yet, or everything
// here still waiting on a reviewer.
import { useMemo } from "react";
import type { ConceptDataSource } from "@neokapi/concept-ui";
import { ConceptList } from "@neokapi/concept-ui";
import { Layers, Shield } from "../../components/icons";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import { ContextHub } from "../shell/ContextHub";
import { createRestConceptSource } from "./restConceptSource";
import { PendingConceptChanges, usePendingConceptChanges } from "./PendingConceptChanges";

export interface ConceptsSectionProps {
  /** Open a concept's dashboard. */
  onOpenConcept: (conceptId: string) => void;
  /** Open a change-set's review page. */
  onOpenChangeSet?: (changesetId: string) => void;
}

export function ConceptsSection({ onOpenConcept, onOpenChangeSet }: ConceptsSectionProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const source: ConceptDataSource = useMemo(() => createRestConceptSource(api, ws), [api, ws]);
  const { pending } = usePendingConceptChanges();

  return (
    <ContextHub
      title="Concepts"
      description="The language-neutral units of your content, each with its terms, status by locale, and direct relations."
      width="wide"
      toolbar={<PendingConceptChanges onOpenChangeSet={onOpenChangeSet} />}
    >
      <ConceptList
        source={source}
        onOpen={onOpenConcept}
        emptyState={<ConceptsZeroState pendingCount={pending.length} />}
      />
    </ContextHub>
  );
}

/**
 * The two empty Concepts pages, which mean opposite things. With nothing waiting,
 * the workspace has no vocabulary yet and the next move is to add or push one.
 * With a change-set in review, the vocabulary exists and is one approval away —
 * saying "no concepts match" there is what made a page with 57 proposed concepts
 * read as a page where nothing had happened.
 */
function ConceptsZeroState({ pendingCount }: { pendingCount: number }) {
  const waiting = pendingCount > 0;
  return (
    <div className="flex flex-col items-center gap-1.5 rounded-xl border border-dashed px-6 py-12 text-center">
      <span className="flex size-9 items-center justify-center rounded-full bg-muted text-muted-foreground [&_svg]:size-5">
        {waiting ? <Shield /> : <Layers />}
      </span>
      <p className="text-sm font-medium">
        {waiting ? "Every concept here is waiting on a review" : "No concepts yet"}
      </p>
      <p className="max-w-sm text-sm text-muted-foreground">
        {waiting
          ? "Proposed concepts join this list once their change is approved. The change waiting for review is above."
          : "Concepts arrive when you add one here or push terms from a project."}
      </p>
    </div>
  );
}
